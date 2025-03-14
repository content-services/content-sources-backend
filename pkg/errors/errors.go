package errors

import "fmt"

var ErrCertKeyNotFound = fmt.Errorf("certificate and key not found")
