package mocks

import (
	"github.com/content-services/yummy/pkg/yum"
	"github.com/stretchr/testify/mock"
)

type YumRepositoryMock struct {
	mock.Mock
}

func (r *YumRepositoryMock) Configure(settings yum.YummySettings) {
}

func (r *YumRepositoryMock) Clear() {}

func (r *YumRepositoryMock) Repomd() (*yum.Repomd, int, error) {
	var repomd *yum.Repomd
	args := r.Called()
	if v, ok := args.Get(0).(*yum.Repomd); ok {
		repomd = v
	}
	return repomd, args.Int(1), args.Error(2)
}

func (r *YumRepositoryMock) Packages() ([]yum.Package, int, error) {
	var packages []yum.Package
	args := r.Called()
	if v, ok := args.Get(0).([]yum.Package); ok {
		packages = v
	}
	return packages, args.Int(1), args.Error(2)
}

func (r *YumRepositoryMock) Signature() (*string, int, error) {
	var signature *string
	args := r.Called()
	if v, ok := args.Get(0).(*string); ok {
		signature = v
	}
	return signature, args.Int(1), args.Error(2)
}
