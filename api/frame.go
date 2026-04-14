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
var inFrame=window!==window.top;
var D=/*DEPLOYS*/[];var A='/*ACTIVE*/';var B='/*BASE*/';
var h=document.createElement('div');
h.id='velori-frame';
var s=h.attachShadow({mode:'closed'});
s.innerHTML='<style>@font-face{font-family:Geist;font-style:normal;font-weight:400 700;font-display:swap;src:url(https://cdn.jsdelivr.net/fontsource/fonts/geist-sans@latest/latin-400-normal.woff2) format("woff2")}:host{all:initial;position:fixed;top:0;left:0;right:0;height:40px;z-index:2147483647;font-family:Geist,Inter,system-ui,-apple-system,sans-serif;pointer-events:auto;text-rendering:optimizeLegibility;-webkit-font-smoothing:antialiased;-moz-osx-font-smoothing:grayscale}.bar{display:flex;align-items:center;height:40px;background:oklch(0.9753 0.0039 107.3/70%%);-webkit-backdrop-filter:blur(40px) saturate(1.5);backdrop-filter:blur(40px) saturate(1.5);color:#18181b;padding:0 12px;font-size:13px;border-bottom:1px solid #e4e4e7}.logo{color:#18181b;text-decoration:none;font-weight:600;font-size:14px;letter-spacing:.02em;flex-shrink:0}.logo:hover{opacity:.7}.center{flex:1;display:flex;align-items:center;justify-content:center;overflow:hidden}.center-inner{display:flex;flex-direction:column;align-items:center;position:relative;cursor:pointer;padding:2px 8px;border-radius:6px;transition:background .1s}.center-inner:hover{background:rgba(0,0,0,.04)}.center .name{font-weight:500;font-size:13px;color:#18181b;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;line-height:1.2}.center .ver{color:#71717a;font-size:11px;line-height:1.2;display:flex;align-items:center;gap:3px}.center .chevron{color:#a1a1aa}.center select{position:absolute;top:0;left:0;width:100%%;height:100%%;opacity:0;cursor:pointer;-webkit-appearance:none;appearance:none}.actions{display:flex;align-items:center;gap:4px;flex-shrink:0}.actions button{-webkit-appearance:none;appearance:none;background:transparent;border:none;color:#71717a;font-family:inherit;font-size:12px;font-weight:500;cursor:pointer;outline:none;padding:4px 10px;border-radius:6px;transition:background .1s,color .1s}.actions button:hover{background:rgba(0,0,0,.06);color:#18181b}</style><div class="bar"><a class="logo" href="%s">Velori</a><div class="center"><div class="center-inner" id="center"><span class="name">%s</span><span class="ver" id="ver"></span></div></div><div class="actions" id="vn"></div></div>';
var vn=s.getElementById("vn");
var verEl=s.getElementById("ver");
var centerEl=s.getElementById("center");
function mksel(parent,selected){
var sel=document.createElement("select");
D.forEach(function(d,i){var o=document.createElement("option");o.value=d.isCurrent?"":d.id;var t="v"+(i+1);if(d.isCurrent)t+=" (Latest)";if(d.label)t+=" — "+d.label;if(d.time)t+=" — "+d.time;o.textContent=t;if(i===selected)o.selected=true;sel.appendChild(o)});
parent.appendChild(sel);return sel;
}
function deployUrl(v){return v?B+"/_v/"+v+"/":B+"/"}
if(D.length>1){
var activeIdx=A?D.findIndex(function(d){return d.id===A}):D.findIndex(function(d){return d.isCurrent});
if(activeIdx<0)activeIdx=D.length-1;
var sel=mksel(centerEl,activeIdx);
sel.onchange=function(){window.location.href=deployUrl(sel.value)};
function updateVerLabel(){var d=D[sel.selectedIndex];var t="v"+(sel.selectedIndex+1);if(d&&d.isCurrent)t+=" (Latest)";if(d&&d.time)t+=" \u00b7 "+d.time;verEl.innerHTML=t+'<svg class="chevron" width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>'}
updateVerLabel();sel.onchange=function(){updateVerLabel();window.location.href=deployUrl(sel.value)};
if(inFrame){
var exitBtn=document.createElement("button");
exitBtn.textContent="Exit";
vn.appendChild(exitBtn);
exitBtn.onclick=function(){window.top.postMessage("velori-exit-split","*")};
}else{
var splitBtn=document.createElement("button");
splitBtn.textContent="Split";
vn.appendChild(splitBtn);
var splitEl=null;
function exitSplit(){if(splitEl){splitEl.remove();splitEl=null;h.style.display="";splitBtn.textContent="Split";sel.style.display="";document.body.style.overflow=""}}
splitBtn.onclick=function(){
if(splitEl){exitSplit();return}
splitBtn.textContent="Exit";
sel.style.display="none";
h.style.display="none";
document.body.style.overflow="hidden";
var ci=sel.selectedIndex;
var rightIdx=ci>0?ci-1:(ci<D.length-1?ci+1:ci);
var ifs="flex:1;border:0;height:100%%";
splitEl=document.createElement("div");
splitEl.id="velori-split";
splitEl.style.cssText="position:fixed;top:0;left:0;right:0;bottom:0;display:flex;z-index:2147483646;background:#fff";
var lf=document.createElement("iframe");lf.src=window.location.href;lf.style.cssText=ifs;
var div=document.createElement("div");div.style.cssText="width:1px;background:#e4e4e7";
var rf=document.createElement("iframe");rf.src=deployUrl(D[rightIdx].isCurrent?"":D[rightIdx].id);rf.style.cssText=ifs;
splitEl.appendChild(lf);splitEl.appendChild(div);splitEl.appendChild(rf);
document.body.appendChild(splitEl);
};
window.addEventListener("message",function(e){if(e.data==="velori-exit-split")exitSplit()});
}
}
document.body.style.paddingTop=(parseFloat(getComputedStyle(document.body).paddingTop)||0)+40+'px';
document.body.appendChild(h);
})();</script>`, origin, name)

	// Inject dynamic values via string replacement (safe from fmt.Sprintf format verb issues)
	deploysJSON, _ := json.Marshal(deploys)
	tpl = strings.Replace(tpl, "/*DEPLOYS*/[]", string(deploysJSON), 1)
	tpl = strings.Replace(tpl, "/*ACTIVE*/", html.EscapeString(activeDeployID), 1)
	tpl = strings.Replace(tpl, "/*BASE*/", html.EscapeString(baseURL), 1)

	return tpl
}

func anonFrameSnippet(appOrigin string, claimURL string) string {
	return ""
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
