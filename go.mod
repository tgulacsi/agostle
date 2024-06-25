module github.com/tgulacsi/agostle

require (
	bitbucket.org/zombiezen/gopdf v0.0.0-20190421151423-ab3d04824694
	github.com/KarpelesLab/reflink v1.0.1
	github.com/UNO-SOFT/filecache v0.1.11
	github.com/UNO-SOFT/otel v0.8.5
	github.com/UNO-SOFT/zlog v0.8.3
	github.com/VictoriaMetrics/metrics v1.34.0
	github.com/coreos/go-systemd/v22 v22.5.0
	github.com/gabriel-vasile/mimetype v1.4.4
	github.com/go-kit/kit v0.13.0
	github.com/google/renameio v1.0.1
	github.com/google/renameio/v2 v2.0.0
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/kardianos/service v1.2.2
	github.com/kylelemons/godebug v1.1.0
	github.com/mholt/archiver/v4 v4.0.0-alpha.8
	github.com/oklog/ulid/v2 v2.1.0
	github.com/pdfcpu/pdfcpu v0.8.0
	github.com/peterbourgon/ff/v3 v3.4.0
	github.com/stvp/go-toml-config v0.0.0-20220807175811-1347a3c4169c
	github.com/tgulacsi/go v0.27.5
	github.com/theupdateframework/go-tuf v0.7.0
	github.com/zRedShift/mimemagic v1.2.0
	golang.org/x/image v0.17.0
	golang.org/x/net v0.26.0
	golang.org/x/sync v0.7.0
	golang.org/x/sys v0.21.0
	golang.org/x/text v0.16.0
)

require (
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/bodgit/plumbing v1.3.0 // indirect
	github.com/bodgit/sevenzip v1.5.1 // indirect
	github.com/bodgit/windows v1.0.1 // indirect
	github.com/dgryski/go-linebreak v0.0.0-20180812204043-d8f37254e7d3 // indirect
	github.com/dsnet/compress v0.0.2-0.20210315054119-f66993602bf5 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/hhrutter/lzw v1.0.0 // indirect
	github.com/hhrutter/tiff v1.0.1 // indirect
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/klauspost/pgzip v1.2.6 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/nwaples/rardecode/v2 v2.0.0-beta.2 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rogpeppe/go-internal v1.12.0 // indirect
	github.com/rs/zerolog v1.29.1 // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.8.0 // indirect
	github.com/sloonz/go-qprintable v0.0.0-20210417175225-715103f9e6eb // indirect
	github.com/therootcompany/xz v1.0.1 // indirect
	github.com/ulikunitz/xz v0.5.12 // indirect
	github.com/valyala/fastrand v1.1.0 // indirect
	github.com/valyala/histogram v1.2.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.52.0 // indirect
	go.opentelemetry.io/otel v1.27.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.27.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.27.0 // indirect
	go.opentelemetry.io/otel/metric v1.27.0 // indirect
	go.opentelemetry.io/otel/sdk v1.27.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.27.0 // indirect
	go.opentelemetry.io/otel/trace v1.27.0 // indirect
	go4.org v0.0.0-20230225012048-214862532bf5 // indirect
	golang.org/x/exp v0.0.0-20240613232115-7f521ea00fb8 // indirect
	golang.org/x/term v0.21.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

go 1.21

toolchain go1.21.0

//replace github.com/UNO-SOFT/filecache => ../../UNO-SOFT/filecache

//replace github.com/tgulacsi/go => ../go

//replace github.com/UNO-SOFT/ulog => ../../UNO-SOFT/ulog
