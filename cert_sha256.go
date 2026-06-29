package libv2ray

import (
	"context"
	gotls "crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/apernet/quic-go"
	"github.com/apernet/quic-go/http3"
	xraytls "github.com/xtls/xray-core/transport/internet/tls"
)

const defaultCertSHA256Timeout = 5 * time.Second

type certSHA256Request struct {
	Address    string `json:"address"`
	Port       int    `json:"port"`
	ServerName string `json:"serverName"`
	TimeoutMs  int64  `json:"timeoutMs"`
}

type certSHA256Result struct {
	SHA256 string `json:"sha256"`
	Error  string `json:"error"`
}

// FetchTlsCertSha256 returns the SHA-256 hash of the leaf certificate from a TLS server.
func FetchTlsCertSha256(requestJSON string) string {
	return fetchCertSHA256(requestJSON, fetchTLSCertSHA256)
}

// FetchQuicCertSha256 returns the SHA-256 hash of the leaf certificate from a QUIC/H3 server.
func FetchQuicCertSha256(requestJSON string) string {
	return fetchCertSHA256(requestJSON, fetchQUICCertSHA256)
}

func fetchCertSHA256(requestJSON string, fetcher func(context.Context, certSHA256Request) (string, error)) string {
	request, err := parseCertSHA256Request(requestJSON)
	if err != nil {
		return encodeCertSHA256Result("", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), request.timeout())
	defer cancel()

	sha256, err := fetcher(ctx, request)
	if err != nil {
		return encodeCertSHA256Result("", err)
	}
	return encodeCertSHA256Result(sha256, nil)
}

func parseCertSHA256Request(requestJSON string) (certSHA256Request, error) {
	var request certSHA256Request
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		return request, err
	}

	request.Address = strings.TrimSpace(request.Address)
	request.ServerName = strings.TrimSpace(request.ServerName)
	if request.Address == "" {
		return request, errors.New("address is empty")
	}
	if request.Port <= 0 || request.Port > 65535 {
		return request, fmt.Errorf("invalid port: %d", request.Port)
	}

	return request, nil
}

func fetchTLSCertSHA256(ctx context.Context, request certSHA256Request) (string, error) {
	dialer := &net.Dialer{}
	rawConn, err := dialer.DialContext(ctx, "tcp", request.address())
	if err != nil {
		return "", err
	}
	defer rawConn.Close()

	tlsConn := gotls.Client(rawConn, &gotls.Config{
		ServerName:         request.tlsServerName(),
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2", "http/1.1"},
	})
	defer tlsConn.Close()

	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return "", err
	}
	return certSHA256Hex(tlsConn.ConnectionState().PeerCertificates)
}

func fetchQUICCertSHA256(ctx context.Context, request certSHA256Request) (string, error) {
	conn, err := quic.DialAddr(ctx, request.address(), &gotls.Config{
		ServerName:         request.tlsServerName(),
		InsecureSkipVerify: true,
		NextProtos:         []string{http3.NextProtoH3},
	}, &quic.Config{
		HandshakeIdleTimeout: request.timeout(),
		MaxIdleTimeout:       request.timeout(),
		EnableDatagrams:      true,
	})
	if err != nil {
		return "", err
	}
	defer conn.CloseWithError(0, "")

	return certSHA256Hex(conn.ConnectionState().TLS.PeerCertificates)
}

func certSHA256Hex(certs []*x509.Certificate) (string, error) {
	if len(certs) == 0 || certs[0] == nil {
		return "", errors.New("peer certificate is empty")
	}
	return xraytls.GenerateCertHashHex(certs[0]), nil
}

func (r certSHA256Request) address() string {
	return net.JoinHostPort(r.Address, strconv.Itoa(r.Port))
}

func (r certSHA256Request) timeout() time.Duration {
	if r.TimeoutMs <= 0 {
		return defaultCertSHA256Timeout
	}
	return time.Duration(r.TimeoutMs) * time.Millisecond
}

func (r certSHA256Request) tlsServerName() string {
	if r.ServerName != "" {
		return r.ServerName
	}
	if net.ParseIP(r.Address) == nil {
		return r.Address
	}
	return ""
}

func encodeCertSHA256Result(sha256 string, err error) string {
	result := certSHA256Result{SHA256: sha256}
	if err != nil {
		result.Error = err.Error()
	}

	response, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return `{"sha256":"","error":"failed to encode result"}`
	}
	return string(response)
}
