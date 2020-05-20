package main

import (
	"io"
	"net"
	"time"
	"bytes"
	"crypto/tls"
 

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
			
			
			data := make([]byte, 1024)
			n, err := c.Read(data)
			n = n + 1
			if err != nil{
				logf("Error read : %v", err) //may be ping data				
				return
			}

			isHttp := checkHttp(data) || checkHttps(data)
			if isHttp {
				logf("Http or Https Request from: %v", c.RemoteAddr())
			}
					
			if isHttp && redir == ""{
				logf("Please set the redir value")
				//defer c.Close()
				return
			}

			c, err = l.Accept()
			c.Write(data)
			defer c.Close()		
			//c.(*net.TCPConn).SetKeepAlive(true)
			var dUrl string 
			if isHttp{
				dUrl = redir

			}else{
				c = shadow(c)

				tgt, err := socks.ReadAddr(c)
				if err != nil {
					logf("failed to get target address: %v", err)
					return
				}			
				dUrl = 	tgt.String()
			}

			rc, err := net.Dial("tcp", dUrl)
			if err != nil {
				logf("failed to connect to target: %v", err)
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

// check if http or https 

func checkHttp(src []byte) bool {
	mOption:= []byte("OPTIONS")
	mGet:= []byte("GET")
	mHead:= []byte("HEAD")
	mPost:= []byte("POST")
	mPut:= []byte("PUT")
	mDelete:= []byte("DELETE")
	mTrace:= []byte("TRACE")
	mConnect:= []byte("CONNECT")

	return  bytes.HasPrefix(src, mOption) ||
			bytes.HasPrefix(src, mGet) ||
			bytes.HasPrefix(src, mHead) ||
			bytes.HasPrefix(src, mPost) ||
			bytes.HasPrefix(src, mPut) ||
			bytes.HasPrefix(src, mDelete) ||
			bytes.HasPrefix(src, mTrace) ||
			bytes.HasPrefix(src, mConnect)

}

func checkHttps(src []byte) bool {
	tls30 :=  []byte{22, byte(tls.VersionSSL30 >> 8 & 0xff), byte(tls.VersionSSL30 & 0xff)}
	tls10 :=  []byte{22, byte(tls.VersionTLS10 >> 8 & 0xff), byte(tls.VersionTLS10 & 0xff)}
	tls11 :=  []byte{22, byte(tls.VersionTLS11 >> 8 & 0xff), byte(tls.VersionTLS11 & 0xff)}
	tls12 :=  []byte{22, byte(tls.VersionTLS12 >> 8 & 0xff), byte(tls.VersionTLS12 & 0xff)}

    return bytes.HasPrefix(src, tls12) ||
    	   bytes.HasPrefix(src, tls10) ||
    	   bytes.HasPrefix(src, tls11) ||
    	   bytes.HasPrefix(src, tls30)
}

// func serverHttp(l net.Listener){
// 
// 	hs := &http.Server{
// 		Handler: &anotherHTTPHandler{},
// 	}
// 	if err := hs.Serve(l); err != cmux.ErrListenerClosed {
// 		logf("*** HTTP Listen handler error:%v", err)
// 	}
// }
// type anotherHTTPHandler struct{}
// func (h *anotherHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
// 
// 	host, _, _ := net.SplitHostPort(r.Host)
// 	u := r.URL
// 	u.Host = net.JoinHostPort(host, "443")
// 	u.Scheme="https"
// 	logf("Redirect http to https:%s", u.String())
// 	http.Redirect(w,r,u.String(), http.StatusMovedPermanently)
// 	//http.Redirect(w, r, "https://" + redir, http.StatusMovedPermanently)
// }

func serverHTTP1(l net.Listener, redir string, fromType string) {
	//logf("HTTP Listen request start, redir is:%s", redir)
	//if(fromType == "http"){
	//	defer l.Close()
	//	//redirect to https
	//	hs := &http.Server{
	//		Handler: &anotherHTTPHandler{},
	//	}
	//	if err := hs.Serve(l); err != cmux.ErrListenerClosed {
	//		logf("*** HTTP Listen handler error:%v", err)
	//	}
	//	return
	//}
	//from https
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
	
		c.(*net.TCPConn).SetKeepAlive(true)
	 		
		logf("*****dial to : %s", redir)
		rc, err := net.Dial("tcp", redir)
		if err != nil {
			defer rc.Close()
			logf("*** Http failed to connect to target: %v", err)
			return
		}
		defer rc.Close()
		rc.(*net.TCPConn).SetKeepAlive(true)
	
		//logf("proxy %s <-> %s", c.RemoteAddr(), redir)
		_, _, err = relay(c, rc)		
		//if err != nil {
		//	if err, ok := err.(net.Error); ok && err.Timeout() {
		//		return // ignore i/o timeout
		//	}
		//	//logf("relay error: %v", err)
		//}				

	}()	

}

//redirect to https
//func (h *anotherHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
//
//	host, _, _ := net.SplitHostPort(r.Host)
//	u := r.URL
//	u.Host = net.JoinHostPort(host, "443")
//	u.Scheme="https"
//	logf("Redirect http to https:%s", u.String())
//	http.Redirect(w,r,u.String(), http.StatusMovedPermanently)
//	//http.Redirect(w, r, "https://" + redir, http.StatusMovedPermanently)
//}