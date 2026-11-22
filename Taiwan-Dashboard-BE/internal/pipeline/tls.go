package pipeline

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"time"
)

// 對方的憑證過期但還沒更新時，暫時容許的主機。憑證鏈與網域照常檢查，只放寬到期日。
var expiredCertGrace = map[string]time.Duration{
	"sta.colife.org.tw": 60 * 24 * time.Hour,
}

func verifyPeer(now func() time.Time) func(tls.ConnectionState) error {
	return func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return errors.New("tls: no peer certificate")
		}
		leaf := cs.PeerCertificates[0]
		inter := x509.NewCertPool()
		for _, c := range cs.PeerCertificates[1:] {
			inter.AddCert(c)
		}
		at := now()
		if grace, ok := expiredCertGrace[cs.ServerName]; ok && at.After(leaf.NotAfter) && at.Sub(leaf.NotAfter) <= grace {
			at = leaf.NotAfter.Add(-time.Minute)
		}
		_, err := leaf.Verify(x509.VerifyOptions{DNSName: cs.ServerName, Intermediates: inter, CurrentTime: at})
		return err
	}
}

func newTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // 改由 VerifyConnection 自行完整驗證
		VerifyConnection:   verifyPeer(time.Now),
	}
	return t
}
