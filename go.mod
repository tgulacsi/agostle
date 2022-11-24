module github.com/tgulacsi/agostle

require (
	bitbucket.org/zombiezen/gopdf v0.0.0-20190421151423-ab3d04824694
	github.com/KarpelesLab/reflink v0.0.2
	github.com/UNO-SOFT/filecache v0.0.5
	github.com/UNO-SOFT/otel v0.3.4
	github.com/VictoriaMetrics/metrics v1.23.0
	github.com/coreos/go-systemd/v22 v22.5.0
	github.com/gabriel-vasile/mimetype v1.4.1
	github.com/go-kit/kit v0.12.0
	github.com/go-logr/logr v1.2.3
	github.com/go-logr/zerologr v1.2.2
	github.com/google/renameio v1.0.1
	github.com/h2non/filetype v1.1.3
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/kardianos/service v1.2.2
	github.com/kylelemons/godebug v1.1.0
	github.com/mholt/archiver/v4 v4.0.0-alpha.7
	github.com/oklog/ulid/v2 v2.1.0
	github.com/peterbourgon/ff/v3 v3.3.0
	github.com/rs/zerolog v1.28.0
	github.com/stvp/go-toml-config v0.0.0-20220807175811-1347a3c4169c
	github.com/tgulacsi/go v0.24.1
	github.com/theupdateframework/go-tuf v0.5.1
	golang.org/x/image v0.1.0
	golang.org/x/net v0.1.0
	golang.org/x/sync v0.1.0
	golang.org/x/text v0.4.0
)

require (
	github.com/andybalholm/brotli v1.0.4 // indirect
	github.com/dgryski/go-linebreak v0.0.0-20180812204043-d8f37254e7d3 // indirect
	github.com/dsnet/compress v0.0.2-0.20210315054119-f66993602bf5 // indirect
	github.com/go-kit/log v0.2.0 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/hhrutter/lzw v0.0.0-20190829144645-6f07a24e8650 // indirect
	github.com/hhrutter/tiff v0.0.0-20190829141212-736cae8d0bc7 // indirect
	github.com/klauspost/compress v1.15.5 // indirect
	github.com/klauspost/pgzip v1.2.5 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/nwaples/rardecode/v2 v2.0.0-beta.2 // indirect
	github.com/pdfcpu/pdfcpu v0.3.13 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pierrec/lz4/v4 v4.1.14 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rogpeppe/go-internal v1.8.1 // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.4.0 // indirect
	github.com/sloonz/go-qprintable v0.0.0-20210417175225-715103f9e6eb // indirect
	github.com/therootcompany/xz v1.0.1 // indirect
	github.com/ulikunitz/xz v0.5.10 // indirect
	github.com/valyala/fastrand v1.1.0 // indirect
	github.com/valyala/histogram v1.2.0 // indirect
	go.opentelemetry.io/otel v1.10.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v0.31.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.10.0 // indirect
	go.opentelemetry.io/otel/metric v0.31.0 // indirect
	go.opentelemetry.io/otel/sdk v1.10.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v0.31.0 // indirect
	go.opentelemetry.io/otel/trace v1.10.0 // indirect
	golang.org/x/exp v0.0.0-20221106115401-f9659909a136 // indirect
	golang.org/x/sys v0.2.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

go 1.17

//replace github.com/UNO-SOFT/filecache => ../../UNO-SOFT/filecache

//replace github.com/tgulacsi/go => ../go

//replace github.com/UNO-SOFT/ulog => ../../UNO-SOFT/ulog
