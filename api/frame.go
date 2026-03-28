package main

import (
	"bytes"
	"fmt"
	"html"
	"net/http"
	"strings"
)

func frameSnippet(projectName string, appOrigin string) string {
	name := html.EscapeString(projectName)
	origin := html.EscapeString(appOrigin)

	return fmt.Sprintf(`<script data-velori-frame>(function(){
if(sessionStorage.getItem('velori-frame-dismissed'))return;
var h=document.createElement('div');
h.id='velori-frame';
var s=h.attachShadow({mode:'closed'});
s.innerHTML='<style>@font-face{font-family:Geist;font-style:normal;font-weight:400 700;font-display:swap;src:url(https://cdn.jsdelivr.net/fontsource/fonts/geist-sans@latest/latin-400-normal.woff2) format("woff2")}:host{all:initial;position:fixed;top:0;left:0;right:0;height:36px;z-index:2147483647;font-family:Geist,Inter,system-ui,-apple-system,sans-serif;pointer-events:auto;text-rendering:optimizeLegibility;-webkit-font-smoothing:antialiased;-moz-osx-font-smoothing:grayscale}.bar{display:flex;align-items:center;height:36px;background:#18181b;color:#fff;padding:0 12px;font-size:13px;box-shadow:0 1px 3px rgba(0,0,0,.3)}.logo{color:#fff;text-decoration:none;font-weight:600;letter-spacing:.02em;margin-right:8px}.logo:hover{opacity:.85}.name{flex:1;text-align:center;opacity:.6;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.close{background:none;border:none;color:#fff;cursor:pointer;opacity:.4;font-size:18px;padding:4px 0 4px 8px;line-height:1}.close:hover{opacity:1}</style><div class="bar"><a class="logo" href="%s">Velori</a><span class="name">%s</span><button class="close" aria-label="Dismiss toolbar" title="Dismiss">&#x2715;</button></div>';
var om=document.body.style.marginTop;s.querySelector('.close').onclick=function(){sessionStorage.setItem('velori-frame-dismissed','1');document.body.style.marginTop=(parseFloat(getComputedStyle(document.body).marginTop)||0)-36+'px';h.remove()};
document.body.style.marginTop=(parseFloat(getComputedStyle(document.body).marginTop)||0)+36+'px';
document.body.appendChild(h);
})();</script>`, origin, name)
}

type frameWriter struct {
	http.ResponseWriter
	snippet     string
	buf         bytes.Buffer
	isHTML      bool
	wroteHeader bool
	statusCode  int
}

func newFrameWriter(w http.ResponseWriter, snippet string) *frameWriter {
	return &frameWriter{
		ResponseWriter: w,
		snippet:        snippet,
		statusCode:     http.StatusOK,
	}
}

func (fw *frameWriter) WriteHeader(code int) {
	fw.wroteHeader = true
	fw.statusCode = code
	fw.isHTML = strings.Contains(fw.Header().Get("Content-Type"), "text/html")
	if !fw.isHTML {
		fw.ResponseWriter.WriteHeader(code)
	}
}

func (fw *frameWriter) Write(b []byte) (int, error) {
	if !fw.wroteHeader {
		fw.WriteHeader(fw.statusCode)
	}
	if fw.isHTML {
		return fw.buf.Write(b)
	}
	return fw.ResponseWriter.Write(b)
}

func (fw *frameWriter) Close() {
	if !fw.isHTML || fw.buf.Len() == 0 {
		return
	}

	body := fw.buf.String()

	// Insert snippet before last </body>, or append at end
	lower := strings.ToLower(body)
	if idx := strings.LastIndex(lower, "</body>"); idx >= 0 {
		body = body[:idx] + fw.snippet + body[idx:]
	} else {
		body += fw.snippet
	}

	fw.Header().Del("Content-Length")
	fw.ResponseWriter.WriteHeader(fw.statusCode)
	fmt.Fprint(fw.ResponseWriter, body)
}
