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
var h=document.createElement('div');
h.id='velori-frame';
var s=h.attachShadow({mode:'closed'});
s.innerHTML='<style>@font-face{font-family:Geist;font-style:normal;font-weight:400 700;font-display:swap;src:url(https://cdn.jsdelivr.net/fontsource/fonts/geist-sans@latest/latin-400-normal.woff2) format("woff2")}:host{all:initial;position:fixed;top:0;left:0;right:0;height:24px;z-index:2147483647;font-family:Geist,Inter,system-ui,-apple-system,sans-serif;pointer-events:auto;text-rendering:optimizeLegibility;-webkit-font-smoothing:antialiased;-moz-osx-font-smoothing:grayscale}.bar{display:flex;align-items:center;height:24px;background:color-mix(in oklab,#18181b 70%%,transparent);-webkit-backdrop-filter:blur(40px) saturate(1.5);backdrop-filter:blur(40px) saturate(1.5);color:#fff;padding:0 10px;font-size:11px}.logo{color:#fff;text-decoration:none;font-weight:600;letter-spacing:.02em;margin-right:8px}.logo:hover{opacity:.85}.name{flex:1;text-align:center;opacity:.6;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}</style><div class="bar"><a class="logo" href="%s">Velori</a><span class="name">%s</span></div>';
document.body.style.marginTop=(parseFloat(getComputedStyle(document.body).marginTop)||0)+24+'px';
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
