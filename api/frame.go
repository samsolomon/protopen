package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
)

func frameSnippet(projectName string, appOrigin string, deploys []toolbarDeploy, activeDeployID string, baseURL string) string {
	name := html.EscapeString(projectName)
	origin := html.EscapeString(appOrigin)

	// Build the template with fmt.Sprintf for the two original %s values only
	tpl := fmt.Sprintf(`<script data-velori-frame>(function(){
if(window!==window.top)return;
var D=/*DEPLOYS*/[];var A='/*ACTIVE*/';var B='/*BASE*/';
var h=document.createElement('div');
h.id='velori-frame';
var s=h.attachShadow({mode:'closed'});
s.innerHTML='<style>@font-face{font-family:Geist;font-style:normal;font-weight:400 700;font-display:swap;src:url(https://cdn.jsdelivr.net/fontsource/fonts/geist-sans@latest/latin-400-normal.woff2) format("woff2")}:host{all:initial;position:fixed;top:0;left:0;right:0;height:40px;z-index:2147483647;font-family:Geist,Inter,system-ui,-apple-system,sans-serif;pointer-events:auto;text-rendering:optimizeLegibility;-webkit-font-smoothing:antialiased;-moz-osx-font-smoothing:grayscale}.bar{display:flex;align-items:center;height:40px;background:oklch(0.9753 0.0039 107.3/70%%);-webkit-backdrop-filter:blur(40px) saturate(1.5);backdrop-filter:blur(40px) saturate(1.5);color:#18181b;padding:0 12px;font-size:13px;border-bottom:1px solid #e4e4e7}.logo{color:#18181b;text-decoration:none;font-weight:600;font-size:14px;letter-spacing:.02em;margin-right:10px}.logo:hover{opacity:.7}.name{flex:1;text-align:center;color:#71717a;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.vn{margin-left:auto;display:flex;align-items:center;gap:6px}.vn select,.vn button{-webkit-appearance:none;appearance:none;background:transparent;border:none;color:#18181b;font-family:inherit;font-size:12px;cursor:pointer;outline:none}.vn select{padding:1px 16px 1px 4px;height:20px;background-image:url("data:image/svg+xml,%%3Csvg xmlns=%%27http://www.w3.org/2000/svg%%27 width=%%278%%27 height=%%278%%27%%3E%%3Cpath d=%%27M2 3l2 2 2-2%%27 stroke=%%27%%2318181b%%27 stroke-width=%%271%%27 fill=%%27none%%27/%%3E%%3C/svg%%3E");background-repeat:no-repeat;background-position:right 2px center}.vn select:hover,.vn button:hover{opacity:.6}</style><div class="bar"><a class="logo" href="%s">Velori</a><span class="name">%s</span><div class="vn" id="vn"></div></div>';
var vn=s.getElementById("vn");
function mksel(parent,selected){
var sel=document.createElement("select");
D.forEach(function(d,i){var o=document.createElement("option");o.value=d.isCurrent?"":d.id;var t="v"+(i+1);if(d.isCurrent)t+=" (Latest)";if(d.label)t+=" — "+d.label;if(d.time)t+=" — "+d.time;o.textContent=t;if(i===selected)o.selected=true;sel.appendChild(o)});
parent.appendChild(sel);return sel;
}
function deployUrl(v){return v?B+"/_v/"+v+"/":B+"/"}
if(D.length>1){
var activeIdx=A?D.findIndex(function(d){return d.id===A}):D.findIndex(function(d){return d.isCurrent});
if(activeIdx<0)activeIdx=D.length-1;
var sel=mksel(vn,activeIdx);
sel.onchange=function(){window.location.href=deployUrl(sel.value)};
var splitBtn=document.createElement("button");
splitBtn.textContent="Split";
vn.appendChild(splitBtn);
var splitEl=null;var rsel=null;
splitBtn.onclick=function(){
if(splitEl){splitEl.remove();splitEl=null;if(rsel){rsel.remove();rsel=null}splitBtn.textContent="Split";document.body.style.overflow="";return}
splitBtn.textContent="Exit";
document.body.style.overflow="hidden";
var ci=sel.selectedIndex;
var rightIdx=ci>0?ci-1:(ci<D.length-1?ci+1:ci);
rsel=mksel(vn,rightIdx);
vn.insertBefore(rsel,splitBtn);
var ifs="flex:1;border:0;height:100%%";
splitEl=document.createElement("div");
splitEl.id="velori-split";
splitEl.style.cssText="position:fixed;top:40px;left:0;right:0;bottom:0;display:flex;z-index:2147483646;background:#fff";
var lf=document.createElement("iframe");lf.src=window.location.href;lf.style.cssText=ifs;
var div=document.createElement("div");div.style.cssText="width:1px;background:#e4e4e7";
var rf=document.createElement("iframe");rf.src=deployUrl(rsel.value);rf.style.cssText=ifs;
splitEl.appendChild(lf);splitEl.appendChild(div);splitEl.appendChild(rf);
document.body.appendChild(splitEl);
rsel.onchange=function(){rf.src=deployUrl(rsel.value)};
};
}
document.body.style.marginTop=(parseFloat(getComputedStyle(document.body).marginTop)||0)+40+'px';
document.body.appendChild(h);
})();</script>`, origin, name)

	// Inject dynamic values via string replacement (safe from fmt.Sprintf format verb issues)
	deploysJSON, _ := json.Marshal(deploys)
	tpl = strings.Replace(tpl, "/*DEPLOYS*/[]", string(deploysJSON), 1)
	tpl = strings.Replace(tpl, "/*ACTIVE*/", html.EscapeString(activeDeployID), 1)
	tpl = strings.Replace(tpl, "/*BASE*/", html.EscapeString(baseURL), 1)
	return tpl
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
