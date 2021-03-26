module github.com/tgulacsi/agostle

require (
	bitbucket.org/zombiezen/gopdf v0.0.0-20190421151423-ab3d04824694
	github.com/UNO-SOFT/otel v0.0.7
	github.com/UNO-SOFT/ulog v1.2.0
	github.com/VictoriaMetrics/metrics v1.15.2
	github.com/benbjohnson/clock v1.1.0 // indirect
	github.com/coreos/go-systemd/v22 v22.2.0
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-linebreak v0.0.0-20180812204043-d8f37254e7d3 // indirect
	github.com/disintegration/gift v1.2.1
	github.com/dsnet/compress v0.0.1 // indirect
	github.com/frankban/quicktest v1.11.3 // indirect
	github.com/gabriel-vasile/mimetype v1.1.2
	github.com/go-kit/kit v0.10.0
	github.com/go-logfmt/logfmt v0.5.0 // indirect
	github.com/go-stack/stack v1.8.0
	github.com/golang/snappy v0.0.3 // indirect
	github.com/google/go-cmp v0.5.5 // indirect
	github.com/h2non/filetype v1.1.1
	github.com/hhrutter/lzw v0.0.0-20190829144645-6f07a24e8650 // indirect
	github.com/hhrutter/tiff v0.0.0-20190829141212-736cae8d0bc7 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/kardianos/service v1.2.0
	github.com/kr/pretty v0.2.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/kylelemons/godebug v1.1.0
	github.com/mholt/archiver v3.1.1+incompatible
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/nwaples/rardecode v1.1.0 // indirect
	github.com/oklog/ulid v1.3.1
	github.com/pdfcpu/pdfcpu v0.3.9 // indirect
	github.com/pelletier/go-toml v1.8.1 // indirect
	github.com/peterbourgon/ff/v3 v3.0.0
	github.com/pierrec/lz4 v2.6.0+incompatible // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rakyll/statik v0.1.6 // indirect
	github.com/sloonz/go-qprintable v0.0.0-20160203160305-775b3a4592d5 // indirect
	github.com/stretchr/testify v1.7.0 // indirect
	github.com/stvp/go-toml-config v0.0.0-20170523163211-314328849d78
	github.com/tent/canonical-json-go v0.0.0-20130607151641-96e4ba3a7613 // indirect
	github.com/tgulacsi/go v0.17.0
	github.com/tgulacsi/statik v0.1.3 // indirect
	github.com/theupdateframework/go-tuf v0.0.0-20201230183259-aee6270feb55
	github.com/ulikunitz/xz v0.5.10 // indirect
	github.com/valyala/fastrand v1.0.0 // indirect
	github.com/valyala/histogram v1.1.2 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc v0.11.0 // indirect
	go.opentelemetry.io/otel v0.18.0 // indirect
	go.opentelemetry.io/otel/metric v0.18.0 // indirect
	go.opentelemetry.io/otel/oteltest v0.18.0 // indirect
	go.opentelemetry.io/otel/sdk v0.18.0 // indirect
	go.opentelemetry.io/otel/sdk/export/metric v0.18.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v0.18.0 // indirect
	go.opentelemetry.io/otel/trace v0.18.0 // indirect
	golang.org/x/crypto v0.0.0-20210220033148-5ea612d1eb83 // indirect
	golang.org/x/image v0.0.0-20210220032944-ac19c3e999fb
	golang.org/x/net v0.0.0-20210226172049-e18ecbb05110
	golang.org/x/review v0.0.0-20200911203840-0edcedeff807 // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210309074719-68d13333faf2 // indirect
	golang.org/x/text v0.3.5
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/inconshreveable/log15.v2 v2.0.0-20200109203555-b30bc20e4fd1
	gopkg.in/tylerb/graceful.v1 v1.2.15
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)

go 1.13

exclude (
	go.opentelemetry.io/otel v0.12.0
	go.opentelemetry.io/otel v0.13.0
	go.opentelemetry.io/otel v0.14.0
	go.opentelemetry.io/otel v0.15.0
	go.opentelemetry.io/otel v0.16.0
	go.opentelemetry.io/otel/sdk v0.12.0
	go.opentelemetry.io/otel/sdk v0.13.0
	go.opentelemetry.io/otel/sdk v0.14.0
	go.opentelemetry.io/otel/sdk v0.15.0
	go.opentelemetry.io/otel/sdk v0.16.0
)

//replace github.com/tgulacsi/go => ../go
