package mocks

import (
	"fmt"

	"github.com/stretchr/testify/mock"
)

type ExternalResourceDao struct {
	mock.Mock
}

func (erd *ExternalResourceDao) FetchGpgKey(url string) (string, error) {
	args := erd.Called(url)
	return args.String(), args.Error(1)
}

func (erd *ExternalResourceDao) FetchRepoMd(url string) (*string, int, error) {
	args := erd.Called(url)
	str, ok := args.Get(0).(*string)
	if !ok {
		panic(fmt.Sprintf("assert: arguments: Int(%d) failed because object wasn't correct type: %v", 0, args.Get(0)))
	}
	return str, args.Int(1), args.Error(2)
}

func (erd *ExternalResourceDao) FetchSignature(url string) (*string, int, error) {
	args := erd.Called(url)
	str, ok := args.Get(0).(*string)
	if !ok {
		panic(fmt.Sprintf("assert: arguments: Int(%d) failed because object wasn't correct type: %v", 0, args.Get(0)))
	}
	return str, args.Int(1), args.Error(2)
}
