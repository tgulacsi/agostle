module github.com/tgulacsi/agostle

require (
	bitbucket.org/zombiezen/gopdf v0.0.0-20190421151423-ab3d04824694
	github.com/KarpelesLab/reflink v1.0.2
	github.com/UNO-SOFT/filecache v0.4.0
	github.com/UNO-SOFT/zlog v0.8.6
	github.com/VictoriaMetrics/metrics v1.38.0
	github.com/coreos/go-systemd/v22 v22.7.0
	github.com/gabriel-vasile/mimetype v1.4.13
	github.com/go-kit/kit v0.13.0
	github.com/google/renameio v1.0.1
	github.com/google/renameio/v2 v2.0.2
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/kardianos/service v1.2.2
	github.com/kylelemons/godebug v1.1.0
	github.com/mholt/archives v0.1.5
	github.com/oklog/ulid/v2 v2.1.1
	github.com/pdfcpu/pdfcpu v0.12.1
	github.com/peterbourgon/ff/v4 v4.0.0-beta.1
	github.com/rogpeppe/retry v0.1.0
	github.com/stvp/go-toml-config v0.0.0-20220807175811-1347a3c4169c
	github.com/tgulacsi/go v0.29.2
	github.com/theupdateframework/go-tuf v0.7.0
	github.com/zRedShift/mimemagic v1.2.0
	golang.org/x/image v0.41.0
	golang.org/x/mod v0.35.0
	golang.org/x/net v0.55.0
	golang.org/x/sync v0.20.0
	golang.org/x/sys v0.45.0
	golang.org/x/text v0.37.0
	mvdan.cc/sh/v3 v3.13.1
)

require (
	github.com/STARRY-S/zip v0.2.3 // indirect
	github.com/andybalholm/brotli v1.2.1 // indirect
	github.com/bodgit/plumbing v1.3.0 // indirect
	github.com/bodgit/sevenzip v1.6.1 // indirect
	github.com/bodgit/windows v1.0.1 // indirect
	github.com/carlos7ags/folio v0.9.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/dgryski/go-linebreak v0.0.0-20180812204043-d8f37254e7d3 // indirect
	github.com/dsnet/compress v0.0.2-0.20230904184137-39efe44ab707 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/hhrutter/lzw v1.0.0 // indirect
	github.com/hhrutter/pkcs7 v0.2.2 // indirect
	github.com/hhrutter/tiff v1.0.3 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/klauspost/pgzip v1.2.6 // indirect
	github.com/mattn/go-runewidth v0.0.24 // indirect
	github.com/mikelolasagasti/xz v1.0.1 // indirect
	github.com/minio/minlz v1.1.1 // indirect
	github.com/nwaples/rardecode/v2 v2.2.2 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pierrec/lz4/v4 v4.1.26 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.9.0 // indirect
	github.com/sloonz/go-qprintable v0.0.0-20210417175225-715103f9e6eb // indirect
	github.com/sorairolake/lzip-go v0.3.8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/ulikunitz/xz v0.5.15 // indirect
	github.com/valyala/fastrand v1.1.0 // indirect
	github.com/valyala/histogram v1.2.0 // indirect
	go4.org v0.0.0-20260112195520-a5071408f32f // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f // indirect
	golang.org/x/term v0.43.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

go 1.26

// replace github.com/UNO-SOFT/filecache => ../../UNO-SOFT/filecache

//replace github.com/tgulacsi/go => ../go

//replace github.com/UNO-SOFT/ulog => ../../UNO-SOFT/ulog

replace github.com/peterbourgon/ff/v4 v4.0.0-beta.1 => github.com/UNO-SOFT/ff/v4 v4.0.0-beta.1.us
