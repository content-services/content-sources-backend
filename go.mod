module github.com/content-services/content-sources-backend

go 1.18

require (
	github.com/DATA-DOG/go-sqlmock v1.5.0
	github.com/ProtonMail/go-crypto v0.0.0-20230331115716-d34776aa93ec
	github.com/confluentinc/confluent-kafka-go v1.9.2
	github.com/content-services/yummy v1.0.5
	github.com/getkin/kin-openapi v0.115.0
	github.com/go-openapi/spec v0.20.8 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/go-playground/validator/v10 v10.12.0
	github.com/golang-migrate/migrate/v4 v4.15.2
	github.com/google/uuid v1.3.0
	github.com/invopop/yaml v0.2.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/labstack/echo/v4 v4.10.2
	github.com/labstack/gommon v0.4.0
	github.com/lib/pq v1.10.7
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mpalmer/gorm-zerolog v0.1.0
	github.com/openlyinc/pointy v1.2.0
	github.com/pelletier/go-toml/v2 v2.0.7 // indirect
	github.com/qri-io/jsonschema v0.2.1
	github.com/redhatinsights/app-common-go v1.6.6
	github.com/redhatinsights/platform-go-middlewares v0.20.0
	github.com/rs/zerolog v1.29.0
	github.com/spf13/afero v1.9.5 // indirect
	github.com/spf13/viper v1.15.0
	github.com/stretchr/testify v1.8.4
	github.com/swaggo/swag v1.8.12
	github.com/ziflex/lecho/v3 v3.5.0
	gorm.io/driver/postgres v1.4.8
	gorm.io/gorm v1.24.7-0.20230310094238-cc2d46e5be42
)

require github.com/prometheus/client_golang v1.14.0

require github.com/RedHatInsights/rbac-client-go v1.0.0

require (
	github.com/Shopify/sarama v1.38.1
	github.com/archdx/zerolog-sentry v1.2.0
	github.com/aws/aws-sdk-go-v2 v1.17.7
	github.com/aws/aws-sdk-go-v2/credentials v1.13.18
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.20.7
	github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2 v2.14.0
	github.com/cloudevents/sdk-go/v2 v2.14.0
	github.com/content-services/zest/release/v2023 v2023.7.1689970066
	github.com/getsentry/sentry-go v0.12.0
	github.com/jackc/pgconn v1.8.0
	github.com/jackc/pgx/v4 v4.10.1
	github.com/jackc/pgx/v5 v5.3.0
	github.com/kr/pretty v0.3.0
	github.com/lzap/cloudwatchwriter2 v1.1.0
	github.com/redis/go-redis/v9 v9.0.4
	go.uber.org/goleak v1.2.1
	golang.org/x/exp v0.0.0-20230321023759-10a507213a29
)

require (
	github.com/eapache/go-resiliency v1.3.0 // indirect
	github.com/eapache/go-xerial-snappy v0.0.0-20230111030713-bf00bc1b83b6 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/h2non/filetype v1.1.3 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.3 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.16.5 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pierrec/lz4/v4 v4.1.17 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475 // indirect
	github.com/rogpeppe/go-internal v1.10.0 // indirect
	github.com/ulikunitz/xz v0.5.11 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	go.uber.org/zap v1.21.0 // indirect
)

require (
	github.com/KyleBanks/depth v1.2.1 // indirect
	github.com/RedHatInsights/event-schemas-go v1.0.3
	github.com/RedHatInsights/insights-operator-utils v1.24.7
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.25 // indirect
	github.com/aws/smithy-go v1.13.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cloudflare/circl v1.3.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/golang-jwt/jwt v3.2.2+incompatible // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/jackc/chunkreader/v2 v2.0.1 // indirect
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgproto3/v2 v2.0.7 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/pgtype v1.6.2 // indirect
	github.com/jackc/puddle v1.1.3 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/leodido/go-urn v1.2.2 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.18 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/oleiade/lane/v2 v2.0.0 // indirect
	github.com/perimeterx/marshmallow v1.1.4 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.42.0 // indirect
	github.com/prometheus/procfs v0.9.0 // indirect
	github.com/qri-io/jsonpointer v0.1.1 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	github.com/subosito/gotenv v1.4.2 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	golang.org/x/crypto v0.7.0 // indirect
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/sys v0.6.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.7.0 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
