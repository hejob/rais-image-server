package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"mime"
	"net/http"
	"net/url"
	"rais/src/iiif"
	"rais/src/img"
	"rais/src/plugins"
	"regexp"
	"strconv"
	"strings"
)

func acceptsLD(req *http.Request) bool {
	for _, h := range req.Header["Accept"] {
		for _, accept := range strings.Split(h, ",") {
			if accept == "application/ld+json" {
				return true
			}
		}
	}

	return false
}

// DZITileSize defines deep zoom tile size
const DZITileSize = 1024

// DZI "constants" - these can be declared once (unlike IIIF regexes) because
// we aren't allowing a custom DZI handler path
var (
	DZIInfoRegex     = regexp.MustCompile(`^/images/dzi/(.+).dzi$`)
	DZITilePathRegex = regexp.MustCompile(`^/images/dzi/(.+)_files/(\d+)/(\d+)_(\d+).jpg$`)
)

// ImageHandler responds to a IIIF URL request and parses the requested
// transformation within the limits of the handler's capabilities
type ImageHandler struct {
	IIIFBase   *url.URL
	FeatureSet *iiif.FeatureSet
	TilePath   string
	Maximums   img.Constraint
}

// NewImageHandler sets up a base ImageHandler with no features
func NewImageHandler(tp string) *ImageHandler {
	return &ImageHandler{
		TilePath: tp,
		Maximums: img.Constraint{Width: math.MaxInt32, Height: math.MaxInt32, Area: math.MaxInt64},
	}
}

// EnableIIIF sets up regexes for IIIF responses
func (ih *ImageHandler) EnableIIIF(u *url.URL) {
	ih.IIIFBase = u
	ih.FeatureSet = iiif.AllFeatures()
}

// cacheKey returns a key for caching if a given IIIF URL is cacheable by our
// current, somewhat restrictive, rules
func cacheKey(u *iiif.URL) string {
	if tileCache != nil && u.Format == iiif.FmtJPG && u.Size.W > 0 && u.Size.W <= 1024 && u.Size.H <= 1024 {
		return u.Path
	}
	return ""
}

// IIIFRoute takes an HTTP request and parses it to see what (if any) IIIF
// translation is requested
func (ih *ImageHandler) IIIFRoute(w http.ResponseWriter, req *http.Request) {
	// Pull identifier from base so we know if we're even dealing with a valid
	// file in the first place
	var url = *req.URL
	url.RawQuery = ""
	p := url.String()

	// If it didn't even match the base, something weird happened, so we just
	// spit out a generic 404
	prefix := ih.IIIFBase.Path + "/"
	if !strings.Contains(p, prefix) {
		http.NotFound(w, req)
		return
	}

	var urlPath = strings.Replace(p, prefix, "", 1)
	iiifURL, err := iiif.NewURL(urlPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid IIIF request %q: %s", iiifURL.Path, err), 400)
		return
	}

	// Check for base path and redirect if that's all we have
	if iiifURL.BaseURIRedirect {
		http.Redirect(w, req, p+"/info.json", 303)
		return
	}

	// Handle info.json prior to reading the image, in case of cached info
	fp := ih.getIIIFPath(iiifURL.ID)
	info, e := ih.getInfo(iiifURL.ID, fp)
	if e != nil {
		if e.Code != 404 {
			Logger.Errorf("Error getting IIIF info.json for resource %s (path %s): %s", iiifURL.ID, fp, e.Message)
		}
		http.Error(w, e.Message, e.Code)
		return
	}

	if iiifURL.Info {
		ih.Info(w, req, info)
		return
	}

	// Check the cache before spending the cycles to read in the image.  For now
	// the cache is very limited to ensure only relatively small requests are
	// actually cached.
	if key := cacheKey(iiifURL); key != "" {
		stats.TileCache.Get()
		data, ok := tileCache.Get(key)
		if ok {
			stats.TileCache.Hit()
			w.Header().Set("Content-Type", mime.TypeByExtension("."+string(iiifURL.Format)))
			w.Write(data.([]byte))
			return
		}
	}

	// No info path should mean a full command path - start reading the image
	res, err := img.NewResource(iiifURL.ID, fp)
	if err != nil {
		e := newImageResError(err)
		if e.Code != 404 {
			Logger.Errorf("Error initializing resource %s (path %s): %s", iiifURL.ID, fp, err)
		}
		http.Error(w, e.Message, e.Code)
		return
	}

	if !iiifURL.Valid() {
		// This means the URI was probably a command, but had an invalid syntax
		http.Error(w, "Invalid IIIF request: "+iiifURL.Error().Error(), 400)
		return
	}

	// Attempt to run the command
	ih.Command(w, req, iiifURL, res, info)
}

func (ih *ImageHandler) getIIIFPath(id iiif.ID) string {
	for _, idtopath := range idToPathPlugins {
		fp, err := idtopath(id)
		if err == nil {
			return fp
		}
		if err == plugins.ErrSkipped {
			continue
		}
		Logger.Warnf("Error trying to use plugin to translate iiif.ID: %s", err)
	}
	return ih.TilePath + "/" + string(id)
}

func convertStrings(s1, s2, s3 string) (i1, i2, i3 int, err error) {
	i1, err = strconv.Atoi(s1)
	if err != nil {
		return
	}
	i2, err = strconv.Atoi(s2)
	if err != nil {
		return
	}
	i3, err = strconv.Atoi(s3)
	return
}

// DZIRoute takes an HTTP request and parses for responding to DZI info and
// tile requests
func (ih *ImageHandler) DZIRoute(w http.ResponseWriter, req *http.Request) {
	p := req.RequestURI

	var id iiif.ID
	var filePath string
	var handler func(*img.Resource)

	parts := DZIInfoRegex.FindStringSubmatch(p)
	if parts != nil {
		id = iiif.URLToID(parts[1])
		filePath = ih.TilePath + "/" + string(id)
		handler = func(r *img.Resource) { ih.DZIInfo(w, r) }
	}

	parts = DZITilePathRegex.FindStringSubmatch(p)
	if parts != nil {
		id = iiif.URLToID(parts[1])
		filePath = ih.TilePath + "/" + string(id)

		level, tileX, tileY, err := convertStrings(parts[2], parts[3], parts[4])
		if err != nil {
			http.Error(w, "Invalid DZI request", 400)
			return
		}

		handler = func(r *img.Resource) { ih.DZITile(w, req, r, level, tileX, tileY) }
	}

	if handler == nil {
		// Some kind of invalid request - just spit out a generic 404
		http.NotFound(w, req)
		return
	}

	res, err := img.NewResource(id, filePath)
	if err != nil {
		e := newImageResError(err)
		if e.Code != 404 {
			Logger.Errorf("Error initializing resource %s (path %s): %s", id, filePath, err)
		}
		http.Error(w, e.Message, e.Code)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	handler(res)
}

// DZIInfo returns the info response for a deep-zoom request.  This is very
// hard-coded because the XML is simple, and deep-zoom is really a minor
// addition to RAIS.
func (ih *ImageHandler) DZIInfo(w http.ResponseWriter, res *img.Resource) {
	format := `<?xml version="1.0" encoding="UTF-8"?>
		<Image xmlns="http://schemas.microsoft.com/deepzoom/2008" TileSize="%d" Overlap="0" Format="jpg">
			<Size Width="%d" Height="%d"/>
		</Image>`
	d := res.Decoder
	xml := fmt.Sprintf(format, DZITileSize, d.GetWidth(), d.GetHeight())
	w.Write([]byte(xml))
}

// DZITile returns a tile by setting up the appropriate IIIF data structure
// based on the deep-zoom request
func (ih *ImageHandler) DZITile(w http.ResponseWriter, req *http.Request, res *img.Resource, l, tX, tY int) {
	// We always return 256x256 or bigger
	if l < 8 {
		l = 8
	}

	// Figure out max level
	d := res.Decoder
	srcWidth := uint64(d.GetWidth())
	srcHeight := uint64(d.GetHeight())

	var maxDimension uint64
	if srcWidth > srcHeight {
		maxDimension = srcWidth
	} else {
		maxDimension = srcHeight
	}
	maxLevel := uint64(math.Ceil(math.Log2(float64(maxDimension))))

	// Ignore absurd levels - even above 20 is pretty unlikely, but this is
	// called "future-proofing".  Or something.
	if uint64(l) > maxLevel {
		http.Error(w, fmt.Sprintf("Image doesn't support zoom level %d", l), 400)
		return
	}

	// Construct a IIIF URL so we can just reuse the IIIF-centric Command function
	var reduction uint64 = 1 << (maxLevel - uint64(l))

	width := reduction * DZITileSize
	height := reduction * DZITileSize
	left := uint64(tX) * width
	top := uint64(tY) * height

	// Fail early if the tile requested isn't legit
	if tX < 0 || tY < 0 || left > srcWidth || top > srcHeight {
		http.Error(w, "Invalid tile request", 400)
		return
	}

	// If any dimension is outside the image area, we have to adjust the requested image and tilesize
	finalWidth := width
	finalHeight := height
	if left+width > srcWidth {
		finalWidth = srcWidth - left
	}
	if top+height > srcHeight {
		finalHeight = srcHeight - top
	}

	requestedWidth := DZITileSize * finalWidth / width

	u, _ := iiif.NewURL(fmt.Sprintf("%s/%d,%d,%d,%d/%d,/0/default.jpg",
		url.QueryEscape(res.FilePath), left, top, finalWidth, finalHeight, requestedWidth))
	ih.Command(w, req, u, res, nil)
}

// Info responds to a IIIF info request with appropriate JSON based on the
// image's data and the handler's capabilities
func (ih *ImageHandler) Info(w http.ResponseWriter, req *http.Request, info *iiif.Info) {
	// Convert info to JSON
	json, err := marshalInfo(info)
	if err != nil {
		http.Error(w, err.Message, err.Code)
		return
	}

	// Set headers - content type is dependent on client
	ct := "application/json"
	if acceptsLD(req) {
		ct = "application/ld+json"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(json)
}

func newImageResError(err error) *HandlerError {
	switch err {
	case img.ErrDimensionsExceedLimits:
		return NewError(err.Error(), 501)
	case img.ErrDoesNotExist:
		return NewError("image resource does not exist", 404)
	default:
		return NewError(err.Error(), 500)
	}
}

func (ih *ImageHandler) getInfo(id iiif.ID, fp string) (info *iiif.Info, err *HandlerError) {
	// Check for cached image data first, and use that to create JSON
	info = ih.loadInfoFromCache(id)

	// Next, check for an overridden info.json file, and just spit that out
	// directly if it exists
	if info == nil {
		info = ih.loadInfoOverride(id, fp)
	}

	if info == nil {
		info, err = ih.loadInfoFromImageResource(id, fp)
	}

	return info, err
}

func (ih *ImageHandler) loadInfoFromCache(id iiif.ID) *iiif.Info {
	if infoCache == nil {
		return nil
	}

	stats.InfoCache.Get()
	data, ok := infoCache.Get(id)
	if !ok {
		return nil
	}

	stats.InfoCache.Hit()
	return ih.buildInfo(id, data.(ImageInfo))
}

func (ih *ImageHandler) loadInfoOverride(id iiif.ID, fp string) *iiif.Info {
	// If an override file isn't found or has an error, just skip it
	var infofile = fp + "-info.json"
	data, err := ioutil.ReadFile(infofile)
	if err != nil {
		return nil
	}

	Logger.Debugf("Loading image data from override file (%s)", infofile)

	// If an override file *is* found, replace the id
	escapedID := ih.IIIFBase.String() + "/" + id.Escaped()
	d2 := bytes.Replace(data, []byte("%ID%"), []byte(escapedID), 1)

	info := new(iiif.Info)
	err = json.Unmarshal(d2, info)
	if err != nil {
		Logger.Errorf("Cannot parse JSON override file %q: %s", infofile, err)
		return nil
	}
	return info
}

func (ih *ImageHandler) loadInfoFromImageResource(id iiif.ID, fp string) (*iiif.Info, *HandlerError) {
	Logger.Debugf("Loading image data from image resource (id: %s)", id)
	res, err := img.NewResource(id, fp)
	if err != nil {
		return nil, newImageResError(err)
	}

	d := res.Decoder
	imageInfo := ImageInfo{
		Width:      d.GetWidth(),
		Height:     d.GetHeight(),
		TileWidth:  d.GetTileWidth(),
		TileHeight: d.GetTileHeight(),
		Levels:     d.GetLevels(),
	}

	if infoCache != nil {
		stats.InfoCache.Set()
		infoCache.Add(id, imageInfo)
	}
	return ih.buildInfo(id, imageInfo), nil
}

func (ih *ImageHandler) buildInfo(id iiif.ID, i ImageInfo) *iiif.Info {
	info := ih.FeatureSet.Info()
	info.Width = i.Width
	info.Height = i.Height

	if ih.Maximums.SmallerThanAny(i.Width, i.Height) {
		info.Profile.MaxArea = ih.Maximums.Area
		info.Profile.MaxWidth = ih.Maximums.Width
		info.Profile.MaxHeight = ih.Maximums.Height
	}

	// Compute scaling and tiling
	if i.TileWidth > 0 && i.TileWidth < i.Width && i.TileHeight < i.Height {
		var sf []int
		scale := 1
		for x := 0; x < i.Levels; x++ {
			// For sanity's sake, let's not tell viewers they can get at absurdly
			// small sizes
			if info.Width/scale < 16 {
				break
			}
			if info.Height/scale < 16 {
				break
			}
			sf = append(sf, scale)
			scale <<= 1
		}

		info.Tiles = make([]iiif.TileSize, 1)
		info.Tiles[0] = iiif.TileSize{
			Width:        i.TileWidth,
			ScaleFactors: sf,
		}
		if i.TileHeight > 0 {
			info.Tiles[0].Height = i.TileHeight
		}
	}

	// The info id is actually the full URL to the resource, not just its ID
	info.ID = ih.IIIFBase.String() + "/" + id.Escaped()
	return info
}

func marshalInfo(info *iiif.Info) ([]byte, *HandlerError) {
	json, err := json.Marshal(info)
	if err != nil {
		Logger.Errorf("Unable to marshal IIIFInfo response: %s", err)
		return nil, NewError("server error", 500)
	}

	return json, nil
}

// Command handles image processing operations
func (ih *ImageHandler) Command(w http.ResponseWriter, req *http.Request, u *iiif.URL, res *img.Resource, info *iiif.Info) {
	// Send last modified time
	if err := sendHeaders(w, req, res.FilePath); err != nil {
		return
	}

	// Do we support this request?  If not, return a 501
	if !ih.FeatureSet.Supported(u) {
		http.Error(w, "Feature not supported", 501)
		return
	}

	var max = ih.Maximums

	// If we have an info, we can make use of it for the constraints rather than
	// using the global constraints; this is useful for overridden info.json files
	if info != nil {
		max = img.Constraint{
			Width:  info.Profile.MaxWidth,
			Height: info.Profile.MaxHeight,
			Area:   info.Profile.MaxArea,
		}
		if max.Width == 0 {
			max.Width = math.MaxInt32
		}
		if max.Height == 0 {
			max.Height = math.MaxInt32
		}
		if max.Area == 0 {
			max.Area = math.MaxInt64
		}
	}
	img, err := res.Apply(u, max)
	if err != nil {
		e := newImageResError(err)
		Logger.Errorf("Error applying transorm: %s", err)
		http.Error(w, e.Message, e.Code)
		return
	}

	w.Header().Set("Content-Type", mime.TypeByExtension("."+string(u.Format)))

	cacheBuf := bytes.NewBuffer(nil)
	if err := EncodeImage(cacheBuf, img, u.Format); err != nil {
		http.Error(w, "Unable to encode", 500)
		Logger.Errorf("Unable to encode to %s: %s", u.Format, err)
		return
	}

	if key := cacheKey(u); key != "" {
		stats.TileCache.Set()
		tileCache.Add(key, cacheBuf.Bytes())
	}

	if _, err := io.Copy(w, cacheBuf); err != nil {
		Logger.Errorf("Unable to encode to %s: %s", u.Format, err)
		return
	}
}
