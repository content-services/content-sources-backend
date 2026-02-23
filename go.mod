module github.com/content-services/content-sources-backend

go 1.24.0

require (
	github.com/ProtonMail/go-crypto v1.3.0
	github.com/content-services/lecho/v3 v3.5.2
	github.com/content-services/tang v0.0.17
	github.com/content-services/yummy v1.0.18
	github.com/getkin/kin-openapi v0.133.0
	github.com/go-openapi/spec v0.22.2 // indirect
	github.com/golang-migrate/migrate/v4 v4.19.1
	github.com/google/uuid v1.6.0
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/labstack/echo/v4 v4.15.1
	github.com/labstack/gommon v0.4.2
	github.com/lib/pq v1.11.2
	github.com/mailru/easyjson v0.9.1 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/redhatinsights/app-common-go v1.6.9
	github.com/rs/zerolog v1.34.0
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.11.1
	github.com/swaggo/swag v1.16.6
	gorm.io/driver/postgres v1.6.0
	gorm.io/gorm v1.31.1
)

require github.com/prometheus/client_golang v1.23.2

require github.com/RedHatInsights/rbac-client-go v1.0.0

require (
	github.com/IBM/sarama v1.46.3
	github.com/archdx/zerolog-sentry v1.8.5
	github.com/aws/aws-sdk-go-v2 v1.41.1
	github.com/aws/aws-sdk-go-v2/config v1.32.9
	github.com/aws/aws-sdk-go-v2/credentials v1.19.9
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.63.1
	github.com/aws/aws-sdk-go-v2/service/s3 v1.96.0
	github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2 v2.16.2
	github.com/cloudevents/sdk-go/v2 v2.16.2
	github.com/content-services/caliri/release/v4 v4.5.0
	github.com/content-services/zest/release/v2025 v2025.12.1766446639
	github.com/getsentry/sentry-go v0.42.0
	github.com/jackc/pgx-zerolog v0.0.0-20230315001418-f978528409eb
	github.com/jackc/pgx/v5 v5.8.0
	github.com/knqyf263/go-rpm-version v0.0.0-20240918084003-2afd7dc6a38f
	github.com/lzap/cloudwatchwriter2 v1.6.0
	github.com/pkg/errors v0.9.1
	github.com/project-kessel/kessel-sdk-go v1.4.0
	github.com/redhatinsights/platform-go-middlewares/v2 v2.0.0
	github.com/redis/go-redis/v9 v9.18.0
	github.com/urfave/cli/v2 v2.27.7
	github.com/xdg/scram v1.0.5
	go.uber.org/goleak v1.3.0
	google.golang.org/grpc v1.79.1
)

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.11-20251209175733-2a1774d88802.1 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.4 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.14 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.41.6 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/eapache/go-resiliency v1.7.0 // indirect
	github.com/eapache/go-xerial-snappy v0.0.0-20230731223053-c322873962e3 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/go-jose/go-jose/v4 v4.1.3 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/swag/conv v0.25.4 // indirect
	github.com/go-openapi/swag/jsonname v0.25.4 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.4 // indirect
	github.com/go-openapi/swag/loading v0.25.4 // indirect
	github.com/go-openapi/swag/stringutils v0.25.4 // indirect
	github.com/go-openapi/swag/typeutils v0.25.4 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.4 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/goccy/go-yaml v1.19.1 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/h2non/filetype v1.1.3 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.4 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.18.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/muhlemmer/gu v0.3.1 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oasdiff/yaml v0.0.0-20250309154309-f31be36b4037 // indirect
	github.com/oasdiff/yaml3 v0.0.0-20250309153720-d2182401db90 // indirect
	github.com/pierrec/lz4/v4 v4.1.23 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20250401214520-65e299d6c5c9 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/ulikunitz/xz v0.5.15 // indirect
	github.com/woodsbury/decimal128 v1.4.0 // indirect
	github.com/xdg/stringprep v1.0.3 // indirect
	github.com/xrash/smetrics v0.0.0-20250705151800-55b8f293f342 // indirect
	github.com/zitadel/logging v0.6.2 // indirect
	github.com/zitadel/oidc/v3 v3.45.1 // indirect
	github.com/zitadel/schema v1.3.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.39.0 // indirect
	go.opentelemetry.io/otel/metric v1.39.0 // indirect
	go.opentelemetry.io/otel/trace v1.39.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/exp v0.0.0-20251209150349-8475f28825e9 // indirect
	golang.org/x/mod v0.31.0 // indirect
	golang.org/x/oauth2 v0.34.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20251213004720-97cd9d5aeac2 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251213004720-97cd9d5aeac2 // indirect
)

require (
	github.com/KyleBanks/depth v1.2.1 // indirect
	github.com/RedHatInsights/event-schemas-go v1.0.6
	github.com/RedHatInsights/insights-operator-utils v1.27.0
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.17 // indirect
	github.com/aws/smithy-go v1.24.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudflare/circl v1.6.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-openapi/jsonpointer v0.22.4 // indirect
	github.com/go-openapi/jsonreference v0.21.4 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.4 // indirect
	github.com/prometheus/procfs v0.19.2 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/time v0.14.0 // indirect
	golang.org/x/tools v0.40.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
