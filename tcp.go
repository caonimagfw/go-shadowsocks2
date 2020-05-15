package main

import (
	"io"
	"net"
	"time"

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

	http.HandleFunc("/", route)

	go http.ListenAndServe(redir, nil)
	logf("http on on %s", redir)

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

			tgt, err := socks.ReadAddr(c)
			if err != nil {
				logf("failed to get target address: %v", err)
				return
			}

			rc, err := net.Dial("tcp", tgt.String())
			if err != nil {
				logf("failed to connect to target: %v", err)
				return
			}
			defer rc.Close()
			rc.(*net.TCPConn).SetKeepAlive(true)

			logf("proxy %s <-> %s", c.RemoteAddr(), tgt)
			_, _, err = relay(c, rc)
			if err != nil {
				if err, ok := err.(net.Error); ok && err.Timeout() {
					return // ignore i/o timeout
				}
				logf("relay error: %v", err)
			}
		}()
	}
}

// relay copies between left and right bidirectionally. Returns number of
// bytes copied from right to left, from left to right, and any error occurred.
func relay(left, right net.Conn) (int64, int64, error) {
	type res struct {
		N   int64
		Err error
	}
	ch := make(chan res)

	go func() {
		n, err := io.Copy(right, left)
		right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
		left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
		ch <- res{n, err}
	}()

	n, err := io.Copy(left, right)
	right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
	left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
	rs := <-ch

	if err == nil {
		err = rs.Err
	}
	return n, rs.N, err
}

//----------------------------------------------------------------------
func route(w http.ResponseWriter, r *http.Request) {
    logf("Addr->%s\tURI->%s\n", r.RemoteAddr, r.URL.Path)
    defer r.Body.Close()
    switch r.URL.Path {
    //case "/": // http forword
    //    w.Write([]byte("welcome to work-stacks"))
    //case "/gettcp": // tcp handle
    //    gettcp(w, r)
    default:
        //http.NotFound(w, r)
        w.Write([]byte("welcome to work-stacks"))
    }
}

/*
//http listen
func listenHttp(addr string, shadow func(net.Conn) net.Conn){
	http.HandleFunc("/", route)
    err :=http.ListenAndServe(addr, nil) // ：443
    if err != nil {
		logf("failed to listen on %s: %v", addr, err)
		return
	}
}

//handle the route
func route(w http.ResponseWriter, r *http.Request) {
    logf("Addr->%s\tURI->%s\n", r.RemoteAddr, r.URL.Path)
    defer r.Body.Close()
    switch r.URL.Path {
    //case "/": // http forword
    //    w.Write([]byte("welcome to work-stacks"))
    case "/gettcp": // tcp handle
        gettcp(w, r)
    default:
        //http.NotFound(w, r)
        w.Write([]byte("welcome to work-stacks"))
    }
}

func gettcp(w http.ResponseWriter, r *http.Request) {
    //这里返回的*bufio.ReadWriter 没有处理, 生产环境注意要情况bufferd
    conn, _, err := w.(http.Hijacker).Hijack()
    if err != nil {
        logf("获取Hijacks失败:%s\n", err.Error())
        return
    }
    if tcp,ok := conn.(*net.TCPConn);ok {
        tcp.SetKeepAlivePeriod(1 * time.Minute)
    }
    //然后就可以做自己要做的操作了.
    conn.Close()
}

func tcpRemoteListen(addr string, shadow func(net.Conn) net.Conn){
	l, err := net.Listen("tcp", addr)
	if err != nil {
		logf("failed to listen on %s: %v", addr, err)
		return
	}
}
*/

//----------------------------------------------------------------------