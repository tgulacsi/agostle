module github.com/tgulacsi/agostle

require (
	bitbucket.org/zombiezen/gopdf v0.0.0-20190421151423-ab3d04824694
	github.com/UNO-SOFT/otel v0.0.8
	github.com/UNO-SOFT/ulog v1.2.0
	github.com/VictoriaMetrics/metrics v1.15.2
	github.com/benbjohnson/clock v1.1.0 // indirect
	github.com/coreos/go-systemd/v22 v22.2.0
	github.com/disintegration/gift v1.2.1
	github.com/dsnet/compress v0.0.1 // indirect
	github.com/frankban/quicktest v1.11.3 // indirect
	github.com/gabriel-vasile/mimetype v1.1.2
	github.com/go-kit/kit v0.10.0
	github.com/go-stack/stack v1.8.0
	github.com/golang/snappy v0.0.3 // indirect
	github.com/google/renameio v1.0.0
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
	github.com/stvp/go-toml-config v0.0.0-20170523163211-314328849d78
	github.com/tgulacsi/go v0.18.3
	github.com/theupdateframework/go-tuf v0.0.0-20201230183259-aee6270feb55
	github.com/ulikunitz/xz v0.5.10 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	golang.org/x/crypto v0.0.0-20210220033148-5ea612d1eb83 // indirect
	golang.org/x/image v0.0.0-20210220032944-ac19c3e999fb
	golang.org/x/net v0.0.0-20210226172049-e18ecbb05110
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210309074719-68d13333faf2 // indirect
	golang.org/x/text v0.3.5
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/inconshreveable/log15.v2 v2.0.0-20200109203555-b30bc20e4fd1
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
