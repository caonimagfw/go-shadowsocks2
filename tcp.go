package main

import (
	"io"
	"net"
	//"net/url"
	"time"
	"fmt"
	"github.com/shadowsocks/go-shadowsocks2/socks"
)

// Create a SOCKS server listening on addr and proxy to server.
func socksLocal(addr, server string, shadow func(net.Conn) net.Conn) {
	logf("SOCKS proxy %s <-> %s", addr, server)
	tcpLocal(addr, server, shadow, func(c net.Conn) (socks.Addr, error) { return socks.Handshake(c) })
}

// Create a TCP tunnel from addr to target via server.
func tcpTun(addr, server, target string, shadow func(net.Conn) net.Conn) {
	tgt := socks.ParseAddr(target)
	if tgt == nil {
		logf("invalid target address %q", target)
		return
	}
	logf("TCP tunnel %s <-> %s <-> %s", addr, server, target)
	tcpLocal(addr, server, shadow, func(net.Conn) (socks.Addr, error) { return tgt, nil })
}

// Listen on addr and proxy to server to reach target from getAddr.
func tcpLocal(addr, server string, shadow func(net.Conn) net.Conn, getAddr func(net.Conn) (socks.Addr, error)) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		logf("failed to listen on %s: %v", addr, err)
		return
	}

	for {
		c, err := l.Accept()
		if err != nil {
			logf("failed to accept: %s", err)
			continue
		}

		go func() {
			defer c.Close()
			c.(*net.TCPConn).SetKeepAlive(true)
			tgt, err := getAddr(c)
			if err != nil {

				// UDP: keep the connection until disconnect then free the UDP socket
				if err == socks.InfoUDPAssociate {
					buf := make([]byte, 1)
					// block here
					for {
						_, err := c.Read(buf)
						if err, ok := err.(net.Error); ok && err.Timeout() {
							continue
						}
						logf("UDP Associate End.")
						return
					}
				}

				logf("failed to get target address: %v", err)
				return
			}

			rc, err := net.Dial("tcp", server)
			if err != nil {
				logf("failed to connect to server %v: %v", server, err)
				return
			}
			defer rc.Close()
			rc.(*net.TCPConn).SetKeepAlive(true)
			rc = shadow(rc)

			if _, err = rc.Write(tgt); err != nil {
				logf("failed to send target address: %v", err)
				return
			}

			logf("proxy %s <-> %s <-> %s", c.RemoteAddr(), server, tgt)
			_, _, err = relay(rc, c)
			if err != nil {
				if err, ok := err.(net.Error); ok && err.Timeout() {
					return // ignore i/o timeout
				}
				logf("relay error: %v", err)
			}
		}()
	}
}

// Listen on addr for incoming connections.
func tcpRemote(addr string, redir string, shadow func(net.Conn) net.Conn) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		logf("failed to listen on %s: %v", addr, err)
		return
	}

	logf("listening TCP on %s", addr)
	for {
		c, err := l.Accept()
		if err != nil {
			logf("failed to accept: %v", err)
			continue
		}

		go func() {
			defer c.Close()
			c.(*net.TCPConn).SetKeepAlive(true)
			c = shadow(c)
			//var tgt []byte
			//var err error


			var dUrl string
			tgt, err := socks.ReadAddr(c)

			dUrl = redir
			if err != nil {
				logf("failed to get target address: %v", err)
				if redir != ""{
					dUrl = redir;
					defer c.Close()
					//clientProxy := c.(*net.TCPConn)
									
					
					/*clientProxy, err := net.Dial("tcp", dUrl)
					if err != nil {
						logf("clientproxy dial failed: %v", err)
						return
					}				
					*/	
					 //defer c.Close()
					//c.(*net.TCPConn).SetKeepAlive(true)


					//c.(*net.TCPConn).SetKeepAlive(true)
					//logf("log c dial error %v", err)
					rc, err := net.Dial("tcp", dUrl)
					if err != nil {
						logf("000failed to connect to target: %v", err)
						return
					}
					defer rc.Close()
					rc.(*net.TCPConn).SetKeepAlive(true)
 
					//rc.(*net.TCPConn).SetKeepAlive(true)
					//fmt.Fprint(c, "HTTP/1.1 200 Connection established\r\n\r\n")
					logf("proxy %s <-> %s", rc.RemoteAddr(), dUrl)
					_, _, err = relay(c, rc)
					if err != nil {
						if err, ok := err.(net.Error); ok && err.Timeout() {
							return // ignore i/o timeout
						}
						logf("relay error: %v", err)
					}	

				}else{
					return
				}				
				
			}else{
				dUrl = tgt.String()
				//rc, err := net.Dial("tcp", tgt.String())

				rc, err := net.Dial("tcp", dUrl)
				if err != nil {
					logf("000failed to connect to target: %v", err)
					return
				}
				defer rc.Close()
				rc.(*net.TCPConn).SetKeepAlive(true)

				logf("proxy %s <-> %s", c.RemoteAddr(), dUrl)
				_, _, err = relay(c, rc)
				if err != nil {
					if err, ok := err.(net.Error); ok && err.Timeout() {
						return // ignore i/o timeout
					}
					logf("relay error: %v", err)
				}				
			}


		}()
	}
}

func tcpRemote2(addr string, redir string, shadow func(net.Conn) net.Conn) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		logf("failed to listen on %s: %v", addr, err)
		return
	}

	logf("listening TCP on %s", addr)
	for {
		c, err := l.Accept()
		if err != nil {
			logf("failed to accept: %v", err)
			continue
		}

		go func() {
			defer c.Close()
			//c.(*net.TCPConn).SetKeepAlive(true)
			//c = shadow(c)
			//var tgt []byte
			//var err error


			var dUrl string
			tgt, err := socks.ReadAddr(c)

			dUrl = redir
			if err != nil {
				logf("failed to get target address: %v", err)
				if redir != ""{
					dUrl = redir;
					defer c.Close()
					

					//clientProxy := c.(*net.TCPConn)
									
					
					/*clientProxy, err := net.Dial("tcp", dUrl)
					if err != nil {
						logf("clientproxy dial failed: %v", err)
						return
					}				
					*/	
					 //defer c.Close()
					//c.(*net.TCPConn).SetKeepAlive(true)


					//c.(*net.TCPConn).SetKeepAlive(true)
					//logf("log c dial error %v", err)
					rc, err := net.Dial("tcp", dUrl)
					if err != nil {
						logf("000failed to connect to target: %v", err)
						return
					}

					fmt.Fprint(c, "HTTP/1.1 200 Connection established\r\n\r\n")

					//defer rc.Close()
					//rc.(*net.TCPConn).SetKeepAlive(true)
 
					//rc.(*net.TCPConn).SetKeepAlive(true)
					//fmt.Fprint(c, "HTTP/1.1 200 Connection established\r\n\r\n")
					logf("proxy %s <-> %s", c.RemoteAddr(), dUrl)
					_, _, err = relay(rc, c)
					if err != nil {
						if err, ok := err.(net.Error); ok && err.Timeout() {
							return // ignore i/o timeout
						}
						logf("relay error: %v", err)
					}	

				}else{
					return
				}				
				
			}else{
				dUrl = tgt.String()
				//rc, err := net.Dial("tcp", tgt.String())

				rc, err := net.Dial("tcp", dUrl)
				if err != nil {
					logf("000failed to connect to target: %v", err)
					return
				}
				defer rc.Close()
				rc.(*net.TCPConn).SetKeepAlive(true)

				logf("proxy %s <-> %s", c.RemoteAddr(), dUrl)
				_, _, err = relay(c, rc)
				if err != nil {
					if err, ok := err.(net.Error); ok && err.Timeout() {
						return // ignore i/o timeout
					}
					logf("relay error: %v", err)
				}				
			}


		}()
	}
}
 

// relay copies between left and right bidirectionally. Returns number of
// bytes copied from right to left, from left to right, and any error occurred.
func relay(left, right net.Conn) (int64, int64, error) {
	logf(" begin 11")
	type res struct {
		N   int64
		Err error
	}
	logf(" begin 22")
	ch := make(chan res)

	go func() {
		logf(" begin 33")
		n, err := io.Copy(right, left)
		right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
		left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
		ch <- res{n, err}
		logf(" begin 255")
	}()
	logf(" begin 44")
	n, err := io.Copy(left, right)
	right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
	left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
	rs := <-ch
	logf(" begin 66")
	if err == nil {
		err = rs.Err
	}
	logf(" begin 77")
	return n, rs.N, err
}