package main

import (
	"errors"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"rais/src/iiif"

	"github.com/spf13/viper"
	"golang.org/x/image/tiff"
)

// ErrInvalidEncodeFormat is the error returned when encoding fails due to a
// file format RAIS doesn't support
var ErrInvalidEncodeFormat = errors.New("Unable to encode: unsupported format")

// EncodeImage uses the built-in image libs to write an image to the browser
func EncodeImage(w io.Writer, img image.Image, format iiif.Format) error {
	switch format {
	case iiif.FmtJPG:
		return jpeg.Encode(w, img, &jpeg.Options{Quality: viper.GetInt("JPGQuality")})
	case iiif.FmtPNG:
		return png.Encode(w, img)
	case iiif.FmtGIF:
		return gif.Encode(w, img, &gif.Options{NumColors: 256})
	case iiif.FmtTIF:
		return tiff.Encode(w, img, &tiff.Options{Compression: tiff.Deflate, Predictor: true})
	}

	return ErrInvalidEncodeFormat
}
