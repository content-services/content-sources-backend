package candlepin_client

//go:generate $GO_OUTPUT/mockery  --name CandlepinClient --filename candlepin_client_mock.go --inpackage
type CandlepinClient interface {
	CreateOwner() error
	ImportManifest(filename string) error
	ListContents(ownerKey string) ([]string, error)
}
