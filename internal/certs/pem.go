package certs

import "encoding/pem"

// pemBlock returns the DER of the next CERTIFICATE block and the rest.
func pemBlock(b []byte) ([]byte, []byte) {
	for {
		var block *pem.Block
		block, b = pem.Decode(b)
		if block == nil {
			return nil, nil
		}
		if block.Type == "CERTIFICATE" {
			return block.Bytes, b
		}
	}
}
