package ssh

import (
	"errors"
	"fmt"

	"github.com/projectdiscovery/nuclei/v3/pkg/protocols/common/protocolstate"
	"github.com/zmap/zgrab2/lib/ssh"
)

func memoizedConnectSSHInfoMode(opts *connectOptions) (*ssh.HandshakeLog, error) {
	hash := "connectSSHInfoMode:" + fmt.Sprintf("%#v\n", opts)

	v, err, _ := protocolstate.Memoizer.Do(hash, func() (interface{}, error) {
		return connectSSHInfoMode(opts)
	})
	if err != nil {
		return nil, err
	}
	if value, ok := v.(*ssh.HandshakeLog); ok {
		return value, nil
	}

	return nil, errors.New("could not convert cached ssh handshake log")
}