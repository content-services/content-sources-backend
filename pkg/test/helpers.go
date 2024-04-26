package test

import (
	"github.com/stretchr/testify/mock"
)

func MockCtx() interface{} {
	return mock.AnythingOfType("*context.valueCtx")
}
