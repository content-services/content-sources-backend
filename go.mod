module github.com/content-services/content-sources-backend

go 1.20

require (
	github.com/ProtonMail/go-crypto v0.0.0-20230923063757-afb1ddc0824c
	github.com/content-services/yummy v1.0.9
	github.com/content-services/tang v0.0.1
	github.com/getkin/kin-openapi v0.122.0
	github.com/go-openapi/spec v0.20.12 // indirect
	github.com/go-openapi/swag v0.22.5 // indirect
	github.com/golang-migrate/migrate/v4 v4.17.0
	github.com/google/uuid v1.5.0
	github.com/invopop/yaml v0.2.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/labstack/echo/v4 v4.11.4
	github.com/labstack/gommon v0.4.2
	github.com/lib/pq v1.10.9
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mpalmer/gorm-zerolog v0.1.0
	github.com/openlyinc/pointy v1.2.1
	github.com/pelletier/go-toml/v2 v2.1.1 // indirect
	github.com/redhatinsights/app-common-go v1.6.7
	github.com/redhatinsights/platform-go-middlewares v0.20.0
	github.com/rs/zerolog v1.31.0
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/viper v1.18.2
	github.com/stretchr/testify v1.8.4
	github.com/swaggo/swag v1.16.2
	github.com/ziflex/lecho/v3 v3.5.0
	gorm.io/driver/postgres v1.5.4
	gorm.io/gorm v1.25.5
)

require github.com/prometheus/client_golang v1.18.0

require github.com/RedHatInsights/rbac-client-go v1.0.0

require (
	github.com/Shopify/sarama v1.38.1
	github.com/archdx/zerolog-sentry v1.8.0
	github.com/aws/aws-sdk-go-v2 v1.24.0
	github.com/aws/aws-sdk-go-v2/credentials v1.16.13
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.30.1
	github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2 v2.14.0
	github.com/cloudevents/sdk-go/v2 v2.14.0
	github.com/content-services/zest/release/v2023 v2023.12.1703173361
	github.com/getsentry/sentry-go v0.25.0
	github.com/jackc/pgconn v1.14.1
	github.com/jackc/pgx/v4 v4.18.1
	github.com/jackc/pgx/v5 v5.5.1
	github.com/lzap/cloudwatchwriter2 v1.2.0
	github.com/redis/go-redis/v9 v9.3.1
	github.com/xdg/scram v1.0.5
	go.uber.org/goleak v1.3.0
	golang.org/x/exp v0.0.0-20231214170342-aacd6d4b4611
)

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.5.4 // indirect
	github.com/eapache/go-resiliency v1.5.0 // indirect
	github.com/eapache/go-xerial-snappy v0.0.0-20230731223053-c322873962e3 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/h2non/filetype v1.1.3 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/jackc/pgx-zerolog v0.0.0-20230315001418-f978528409eb // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.4 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.17.4 // indirect
	github.com/matttproud/golang_protobuf_extensions/v2 v2.0.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/pierrec/lz4/v4 v4.1.19 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475 // indirect
	github.com/sagikazarmark/locafero v0.4.0 // indirect
	github.com/sagikazarmark/slog-shim v0.1.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/ulikunitz/xz v0.5.11 // indirect
	github.com/xdg/stringprep v1.0.3 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	golang.org/x/sync v0.5.0 // indirect
)

require (
	github.com/KyleBanks/depth v1.2.1 // indirect
	github.com/RedHatInsights/event-schemas-go v1.0.6
	github.com/RedHatInsights/insights-operator-utils v1.24.13
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.2.9 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.5.9 // indirect
	github.com/aws/smithy-go v1.19.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cloudflare/circl v1.3.7 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-openapi/jsonpointer v0.20.1 // indirect
	github.com/go-openapi/jsonreference v0.20.3 // indirect
	github.com/golang-jwt/jwt v3.2.2+incompatible // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/jackc/chunkreader/v2 v2.0.1 // indirect
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgproto3/v2 v2.3.2 // indirect
	github.com/jackc/pgservicefile v0.0.0-20231201235250-de7065d80cb9 // indirect
	github.com/jackc/pgtype v1.14.0 // indirect
	github.com/jackc/puddle v1.3.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/oleiade/lane/v2 v2.0.0 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.45.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.1 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/crypto v0.17.0 // indirect
	golang.org/x/net v0.19.0 // indirect
	golang.org/x/sys v0.15.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	golang.org/x/tools v0.16.1 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
