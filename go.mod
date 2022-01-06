module github.com/tgulacsi/agostle

require (
	bitbucket.org/zombiezen/gopdf v0.0.0-20190421151423-ab3d04824694
	github.com/UNO-SOFT/otel v0.1.2
	github.com/UNO-SOFT/ulog v1.4.2
	github.com/VictoriaMetrics/metrics v1.15.2
	github.com/benbjohnson/clock v1.1.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2
	github.com/dsnet/compress v0.0.2-0.20210315054119-f66993602bf5 // indirect
	github.com/frankban/quicktest v1.11.3 // indirect
	github.com/gabriel-vasile/mimetype v1.1.2
	github.com/go-kit/kit v0.10.0
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/renameio v1.0.1
	github.com/h2non/filetype v1.1.1
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/kardianos/service v1.2.0
	github.com/kr/text v0.2.0 // indirect
	github.com/kylelemons/godebug v1.1.0
	github.com/mholt/archiver v3.1.1+incompatible
	github.com/nwaples/rardecode v1.1.0 // indirect
	github.com/oklog/ulid/v2 v2.0.2
	github.com/pdfcpu/pdfcpu v0.3.9 // indirect
	github.com/pelletier/go-toml v1.8.1 // indirect
	github.com/peterbourgon/ff/v3 v3.0.0
	github.com/pierrec/lz4 v2.6.0+incompatible // indirect
	github.com/sloonz/go-qprintable v0.0.0-20210417175225-715103f9e6eb // indirect
	github.com/stvp/go-toml-config v0.0.0-20170523163211-314328849d78
	github.com/tgulacsi/go v0.19.3
	github.com/theupdateframework/go-tuf v0.0.0-20201230183259-aee6270feb55
	github.com/ulikunitz/xz v0.5.10 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	golang.org/x/crypto v0.0.0-20210220033148-5ea612d1eb83 // indirect
	golang.org/x/image v0.0.0-20210220032944-ac19c3e999fb
	golang.org/x/net v0.0.0-20211013171255-e13a2654a71e
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20211013075003-97ac67df715c // indirect
	golang.org/x/text v0.3.7
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)

require (
	github.com/UNO-SOFT/filecache v0.0.2
	github.com/go-kit/log v0.2.0
)

require (
	github.com/KarpelesLab/reflink v0.0.2 // indirect
	github.com/andybalholm/brotli v1.0.4 // indirect
	github.com/dgryski/go-linebreak v0.0.0-20180812204043-d8f37254e7d3 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/goccy/go-json v0.8.1 // indirect
	github.com/hhrutter/lzw v0.0.0-20190829144645-6f07a24e8650 // indirect
	github.com/hhrutter/tiff v0.0.0-20190829141212-736cae8d0bc7 // indirect
	github.com/klauspost/compress v1.13.6 // indirect
	github.com/klauspost/pgzip v1.2.5 // indirect
	github.com/mholt/archiver/v3 v3.5.1 // indirect
	github.com/mholt/archiver/v4 v4.0.0-alpha.1 // indirect
	github.com/nwaples/rardecode/v2 v2.0.0-beta.2 // indirect
	github.com/pierrec/lz4/v4 v4.1.12 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rogpeppe/go-internal v1.8.0 // indirect
	github.com/tent/canonical-json-go v0.0.0-20130607151641-96e4ba3a7613 // indirect
	github.com/therootcompany/xz v1.0.1 // indirect
	github.com/valyala/fastrand v1.0.0 // indirect
	github.com/valyala/histogram v1.1.2 // indirect
	go.opentelemetry.io/otel v1.0.1 // indirect
	go.opentelemetry.io/otel/internal/metric v0.24.0 // indirect
	go.opentelemetry.io/otel/metric v0.24.0 // indirect
	go.opentelemetry.io/otel/sdk v1.0.1 // indirect
	go.opentelemetry.io/otel/sdk/export/metric v0.24.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v0.24.0 // indirect
	go.opentelemetry.io/otel/trace v1.0.1 // indirect
	golang.org/x/exp v0.0.0-20210819164307-503510c5c1ec // indirect
)

go 1.17

//replace github.com/UNO-SOFT/filecache => ../../UNO-SOFT/filecache

//replace github.com/tgulacsi/go => ../go

//replace github.com/UNO-SOFT/ulog => ../../UNO-SOFT/ulog
