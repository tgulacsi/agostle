module github.com/tgulacsi/agostle

require (
	bitbucket.org/zombiezen/gopdf v0.0.0-20190421151423-ab3d04824694
	github.com/UNO-SOFT/otel v0.0.5
	github.com/UNO-SOFT/ulog v1.2.0
	github.com/VictoriaMetrics/metrics v1.13.0
	github.com/coreos/go-systemd/v22 v22.1.0
	github.com/disintegration/gift v1.2.1
	github.com/dsnet/compress v0.0.1 // indirect
	github.com/frankban/quicktest v1.5.0 // indirect
	github.com/gabriel-vasile/mimetype v1.1.2
	github.com/go-kit/kit v0.10.0
	github.com/go-stack/stack v1.8.0
	github.com/golang/snappy v0.0.2 // indirect
	github.com/h2non/filetype v1.1.1
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/kardianos/service v1.2.0
	github.com/kylelemons/godebug v1.1.0
	github.com/mholt/archiver v3.1.1+incompatible
	github.com/nwaples/rardecode v1.1.0 // indirect
	github.com/oklog/ulid v1.3.1
	github.com/pdfcpu/pdfcpu v0.3.8 // indirect
	github.com/pelletier/go-toml v1.8.1 // indirect
	github.com/peterbourgon/ff/v3 v3.0.0
	github.com/pierrec/lz4 v2.6.0+incompatible // indirect
	github.com/stvp/go-toml-config v0.0.0-20170523163211-314328849d78
	github.com/tgulacsi/go v0.13.7
	github.com/theupdateframework/go-tuf v0.0.0-20201230183259-aee6270feb55
	github.com/ulikunitz/xz v0.5.10 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad // indirect
	golang.org/x/image v0.0.0-20200927104501-e162460cd6b5
	golang.org/x/net v0.0.0-20201110031124-69a78807bb2b
	golang.org/x/review v0.0.0-20200911203840-0edcedeff807 // indirect
	golang.org/x/sync v0.0.0-20201020160332-67f06af15bc9
	golang.org/x/sys v0.0.0-20210124154548-22da62e12c0c // indirect
	golang.org/x/text v0.3.4
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	gopkg.in/inconshreveable/log15.v2 v2.0.0-20200109203555-b30bc20e4fd1
	gopkg.in/tylerb/graceful.v1 v1.2.15
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
