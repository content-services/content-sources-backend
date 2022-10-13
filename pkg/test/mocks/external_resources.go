package mocks

import (
	"github.com/stretchr/testify/mock"
)

type ExternalResourceDao struct {
	mock.Mock
}

func (erd *ExternalResourceDao) ValidRepoMD(url string) (int, error) {
	args := erd.Called(url)
	return args.Int(0), args.Error(1)
}

func (erd *ExternalResourceDao) FetchGpgKey(url string) (string, error) {
	args := erd.Called(url)
	return args.String(), args.Error(1)
}
