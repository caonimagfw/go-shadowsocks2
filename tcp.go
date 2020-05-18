package main

import (
	"io"
	"net"
	"net/rpc"
	"time"
	"net/http"
	"strings"

	"github.com/shadowsocks/go-shadowsocks2/socks"
	"github.com/soheilhy/cmux"
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

// relay copies between left and right bidirectionally. Returns number of
// bytes copied from right to left, from left to right, and any error occurred.
func relay(left, right net.Conn) (int64, int64, error) {
	//logf(" begin 11")
	type res struct {
		N   int64
		Err error
	}
	ch := make(chan res)

	go func() {
		n, err := io.Copy(right, left)
		//if err != nil {
		//	logf("copy right to left error: %v", err)
		//}	
		right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
		left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
		ch <- res{n, err}
	}()
	n, err := io.Copy(left, right)
	//if err != nil {
	//		logf("copy left to right error: %v", err)
	//}	
	right.SetDeadline(time.Now()) // wake up the other goroutine blocking on right
	left.SetDeadline(time.Now())  // wake up the other goroutine blocking on left
	rs := <-ch
	//logf(" begin 66")
	if err == nil {
		err = rs.Err
	}
	return n, rs.N, err
}

//----------------------------------------------------
// use cmux
func tcpRemotev2(addr string, redir string, shadow func(net.Conn) net.Conn) {
	//create TCP listener
	l, err := net.Listen("tcp", addr)
	if err != nil {
		logf("failed to listen on %s: %v", addr, err)
		return
	}

	logf("listening TCP on %s", addr)

	// Create a mux
	m := cmux.New(l)

	// match list
	httpl := m.Match(cmux.HTTP1Fast())
	tlsl  := m.Match(cmux.TLS())
	tcpl  := m.Match(cmux.Any())


	go serverHTTP1(httpl, redir, "http")
	go serverHTTP1(tlsl, redir, "https")
	go serverTCP(tcpl, redir, shadow)

	if err := m.Serve(); !strings.Contains(err.Error(), "use of closed network connection") {
		panic(err)
		//logf("HTTP Listen handler error:%v", err)
	}
}

type anotherHTTPHandler struct{}

//redirect to https
func (h *anotherHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	host, _, _ := net.SplitHostPort(r.Host)
	u := r.URL
	u.Host = net.JoinHostPort(host, "443")
	u.Scheme="https"
	logf("Redirect http to https:%s", u.String())
	http.Redirect(w,r,u.String(), http.StatusMovedPermanently)
	//http.Redirect(w, r, "https://" + redir, http.StatusMovedPermanently)
}

func serverHTTP1(l net.Listener, redir string, fromType string) {

	logf("HTTP normal request start, redir is:%s", redir)
	if(fromType == "http"){
		//redirect to https
		hs := &http.Server{
			Handler: &anotherHTTPHandler{},
		}
		if err := hs.Serve(l); err != cmux.ErrListenerClosed {
			logf("*** HTTP Listen handler error:%v", err)
		}
		return
	}

	//forward http  
	s := rpc.NewServer()
	if err := s.Register(&RecursiveRPCRcvr{}); err != nil {
		logf("*** serverHTTP1 TCP handler error:%v", err)
	}
	for {
		logf("Receive request type is :%s", fromType)
		c, err := l.Accept()
		if err != nil {
			logf("TCP Accept error:%v", err)
			defer c.Close()
			return
		}
		go func() {
			logf("Http Remote Address %s connected ", c.RemoteAddr())
			defer c.Close()
	 
			m1 := c.(*cmux.MuxConn)
		
			m1.Conn.(*net.TCPConn).SetKeepAlive(true)
		 
			if( redir == "" ){
				logf("*** Http redir not setting ")
				defer c.Close()
				return
			}
		
			rc, err := net.Dial("tcp", redir)
			if err != nil {
				logf("*** Http failed to connect to target: %v", err)
				return
			}
			defer rc.Close()
			rc.(*net.TCPConn).SetKeepAlive(true)
		
			logf("proxy %s <-> %s", c.RemoteAddr(), redir)
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

func serverHTTPS(l net.Listener) {
	logf("HTTPS 2222 normal request %s", " ")
	s := &http.Server{
		Handler: &anotherHTTPHandler{},
	}
	if err := s.Serve(l); err != cmux.ErrListenerClosed {
		logf("HTTPS 2222 Listen handler error:%v", err)
	}

}
type RecursiveRPCRcvr struct{}

func (r *RecursiveRPCRcvr) Cube(i int, j *int) error {
	*j = i * i
	return nil
}

func serverTCP(l net.Listener, redir string, shadow func(net.Conn) net.Conn) {
	s := rpc.NewServer()
	if err := s.Register(&RecursiveRPCRcvr{}); err != nil {
		logf("TCP handler error:%v", err)
	}
	for {
		logf("Receive request type is TCP")
		c, err := l.Accept()
		if err != nil {
			//if err != cmux.ErrListenerClosed {
			//	panic(err)
			//}
			logf("*** TCP Accept error:%v", err)
			return
		}
		go func() {
			logf("Remote Address %s connected ", c.RemoteAddr())
			defer c.Close()
			m1 := c.(*cmux.MuxConn)

			m1.Conn.(*net.TCPConn).SetKeepAlive(true)

			c = shadow(m1)
			//var tgt []byte
			//var err error

			var dUrl string
			tgt, err := socks.ReadAddr(c)

			dUrl = redir
			if err != nil {
				logf("*** failed to get target address: %v", err)	
				if(dUrl == ""){
					//not has redirect 
					return
				}
				logf("Redirect address to %s", redir)
			}else{
				dUrl = tgt.String()
			}
				
			//rc, err := net.Dial("tcp", tgt.String())

			rc, err := net.Dial("tcp", dUrl)
			if err != nil {
				logf("*** failed to connect to target: %v", err)
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
				logf("*** relay error: %v", err)
			}				


		}()

	} //end for
}

func tcpHandler(l net.Listener){
	s := rpc.NewServer()
	if err := s.Register(&RecursiveRPCRcvr{}); err != nil {
		panic(err)
	}
	for {
		conn, err := l.Accept()
		if err != nil {
			if err != cmux.ErrListenerClosed {
				panic(err)
			}
			return
		}
		go s.ServeConn(conn)
	}
}

//----------------------------------------------------