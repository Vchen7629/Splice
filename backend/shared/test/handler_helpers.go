package test

import (
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func FreePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	err = ln.Close()
	require.NoError(t, err)
	return port
}
