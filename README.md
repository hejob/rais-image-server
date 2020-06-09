[![Go Report Card](https://goreportcard.com/badge/github.com/uoregon-libraries/rais-image-server)](https://goreportcard.com/report/github.com/uoregon-libraries/rais-image-server)

Rodent-Assimilated Image Server
=======

![Gocutus, the RAIS mascot](gocutus.png?raw=true "Gocutus, the RAIS mascot")

RAIS was originally built by [eikeon](https://github.com/eikeon) as a 100% open
source, no-commercial-products-required, proof-of-concept tile server for JP2
images within [chronam](https://github.com/LibraryOfCongress/chronam).

It has been updated to allow more command-line options, more source file
formats, more features, and conformance to the [IIIF](http://iiif.io/) spec.

RAIS is very efficient, completely free, and easy to set up and run.  See our
[wiki](https://github.com/uoregon-libraries/rais-image-server/wiki) pages for
more details and documentation.

Configuration
-----

### Main Configuration Settings

RAIS uses a configuration system that allows environment variables, a config
file, and/or command-line flags.  See [rais-example.toml](rais-example.toml)
for an example of a configuration file.  RAIS will use a configuration
file if one exists at `/etc/rais.toml`.

The configuration file's values can be overridden by environment variables,
while command-line flags will override both configuration files and
environtmental variables.  Configuration is best explained and understood by
reading the example file above, which describes all the values in detail.

### Cloud Settings

Because connecting to a cloud provider is optional, often means using a
container-based setup, and differs from one provider to the next, all RAIS
cloud configuration is environment-only.  This means it can't be specified on
the command line or in `rais.toml`.

Currently RAIS can theoretically support S3, Azure, and Google Cloud backends,
but only S3 has had much testing.  To set up RAIS for S3, you would have to
export the following environment variables (in addition to having an
S3-compatible object store running):

- `AWS_ACCESS_KEY_ID`: Required
- `AWS_SECRET_ACCESS_KEY`: Required
- `AWS_REGION`: Required
- `RAIS_S3_ENDPOINT`: optionally set for custom S3 backends; e.g., "minio:9000"
- `RAIS_S3_DISABLESSL`: optionally set this to "true" for custom S3 backends
  which don't need SSL (for instance if they're running on the same server as
  RAIS)
- `RAIS_S3_FORCEPATHSTYLE`: optionally set this to "true" to force path-style
  S3 calls.  This is typically necessary for custom S3 backends like minio, but
  not for AWS.

Other backends have their own environment variables which have to be set in
order to have RAIS connect to them.

For a full demo of a working custom S3 backend powered by minio, see `docker/s3demo`.

IIIF Features
-----

RAIS supports level 2 of the IIIF Image API 2.1 as well as a handful of
features beyond level 2.  See
[the IIIF Features wiki page](https://github.com/uoregon-libraries/rais-image-server/wiki/IIIF-Features)
for an in-depth look at feature support.

Caching
-----

RAIS can internally cache the IIIF `info.json` requests and individual tile
requests.  See the [RAIS Caching](https://github.com/uoregon-libraries/rais-image-server/wiki/Caching)
wiki page for details.

Generating tiled, multi-resolution JP2s
---

RAIS performs best with JP2s which are generated as tiled, multi-resolution
(think "zoom levels") images.  Generating images like this is fairly easy with
either the openjpeg tools or graphicsmagick.  Other tools probably do this
well, but we've only directly used those.

You can find detailed instructions on the
[How to encode jp2s](https://github.com/uoregon-libraries/rais-image-server/wiki/How-To-Encode-JP2s)
wiki page.

License
-----

<img src="http://i.creativecommons.org/p/zero/1.0/88x31.png" style="border-style: none;" alt="CC0" />

RAIS Image Server is in the public domain under a
[CC0](http://creativecommons.org/publicdomain/zero/1.0/) license.

Contributors
-----

Special thanks to Jessica Dussault (@jduss4) for providing the hand-drawn
"Gocutus" logo, and Greg Tunink (@techgique) for various digital refinements to
said logo.
