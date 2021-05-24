# tcp-intercept
Barebones intercepting TCP proxy written in Go. Based off [this gist](https://gist.github.com/ericflo/7dcf4179c315d8bd714c) by [ericflo](https://github.com/ericflo). Intended for use with text-based protocols. Currently will only read and split data by a provided delimeter.

## Example

```
package main

import (
	"fmt"

	"github.com/shellhazard/tcpint"
)

const (
	LocalServerAddress string = "127.0.0.1:1337"
	RemoteServerAddress string = "8.8.8.8:25565"
)

// From our client to our proxy, to our remote host
func handleOutgoing(b []byte) []byte {
	fmt.Println("incoming data:", string(b))
	return b
}

// From the remote host to our proxy, to our client
func handleIncoming(b []byte) []byte {
	fmt.Println("outgoing data:", string(b))
	return b
}

func main() {
	proxy := tcpint.NewProxy(
		LocalServerAddress,
		RemoteServerAddress,
		handleOutgoing,
		handleIncoming,
		tcpint.NULLBYTE
	)

	// Block
	for {
		select{}
	}
}
```