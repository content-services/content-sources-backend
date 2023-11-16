package dao

import "github.com/stretchr/testify/mock"

func (m *MockRepositoryConfigDao) WithContextMock() *MockRepositoryConfigDao {
	m.On("WithContext", mock.AnythingOfType("*context.valueCtx")).Return(m)
	return m
}

func (m *MockSnapshotDao) WithContextMock() *MockSnapshotDao {
	m.On("WithContext", mock.AnythingOfType("*context.valueCtx")).Return(m)
	return m
}
