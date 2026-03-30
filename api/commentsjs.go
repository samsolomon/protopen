package main

import "net/http"

const commentsJS = `(function(){
var host=document.getElementById('velori-frame');
if(!host)return;
var ctx=host.__veloriCtx;
var sr=host.__veloriSR;
if(!ctx||!ctx.userId)return;

var BAR_H=40;
var commentMode=false;
var overlay=null;
var popover=null;
var pins=[];
var cachedComments=[];
var currentPath=ctx.pagePath;

var styleEl=document.createElement('style');
styleEl.textContent=` + "`" + `
[data-vlr-pin]{position:absolute;width:28px;height:28px;border-radius:50%;background:oklch(0.6276 0.2 43.6/90%);-webkit-backdrop-filter:blur(12px) saturate(1.5);backdrop-filter:blur(12px) saturate(1.5);color:#fff;font-size:12px;font-weight:600;display:flex;align-items:center;justify-content:center;cursor:pointer;z-index:2147483640;box-shadow:0 2px 8px rgba(0,0,0,.15);border:2px solid rgba(255,255,255,.6);transform:translate(-50%,-50%);transition:opacity .15s,transform .15s;pointer-events:auto;font-family:system-ui,sans-serif}
[data-vlr-pin]:hover{transform:translate(-50%,-50%) scale(1.15);box-shadow:0 3px 12px rgba(0,0,0,.2)}
[data-vlr-pin].vlr-own{cursor:grab}
[data-vlr-pin].vlr-dragging{opacity:.75;transform:translate(-50%,-50%) scale(1.2);cursor:grabbing;transition:none;z-index:2147483646}
#vlr-overlay{position:fixed;top:40px;left:0;right:0;bottom:0;z-index:2147483639;pointer-events:none}
#vlr-overlay.placing{pointer-events:auto;cursor:crosshair}
.vlr-popover{position:absolute;z-index:2147483645;background:rgba(255,255,255,.92);-webkit-backdrop-filter:blur(20px) saturate(1.5);backdrop-filter:blur(20px) saturate(1.5);border:1px solid #e4e4e7;border-radius:10px;box-shadow:0 4px 24px rgba(0,0,0,.1);width:320px;font-family:Geist,Inter,system-ui,-apple-system,sans-serif;font-size:13px;color:#18181b;overflow:hidden}
.vlr-popover .vlr-pop-body{padding:12px 14px;max-height:320px;overflow-y:auto}
.vlr-popover .vlr-msg{margin-bottom:10px}
.vlr-popover .vlr-msg:last-child{margin-bottom:0}
.vlr-popover .vlr-msg-head{display:flex;align-items:center;gap:6px;margin-bottom:3px}
.vlr-popover .vlr-avatar{width:22px;height:22px;border-radius:50%;background:#18181b;color:#fff;font-size:10px;font-weight:700;display:flex;align-items:center;justify-content:center;flex-shrink:0}
.vlr-popover .vlr-author{font-weight:600;font-size:12px}
.vlr-popover .vlr-time{color:#a1a1aa;font-size:11px}
.vlr-popover .vlr-body{line-height:1.5;white-space:pre-wrap;word-break:break-word}
.vlr-popover .vlr-replies{border-top:1px solid #f4f4f5;margin-top:8px;padding-top:8px}
.vlr-popover .vlr-reply-msg{margin-bottom:8px;padding-left:10px;border-left:2px solid #e4e4e7}
.vlr-popover .vlr-pop-header{display:flex;align-items:center;justify-content:space-between;padding:10px 10px 0 14px}
.vlr-popover .vlr-pop-header-left{display:flex;align-items:center;gap:6px}
.vlr-popover .vlr-toolbar{display:flex;align-items:center;gap:2px;position:relative}
.vlr-popover .vlr-toolbar button{width:26px;height:26px;border-radius:50%;border:none;background:transparent;cursor:pointer;display:flex;align-items:center;justify-content:center;color:#d4d4d8;transition:color .1s}
.vlr-popover .vlr-toolbar button:hover{color:#71717a}
.vlr-popover .vlr-toolbar button.vlr-resolve-active{color:#059669}
.vlr-popover .vlr-toolbar button.vlr-resolve-active:hover{background:#ecfdf5}
.vlr-popover .vlr-menu{position:absolute;top:100%;right:0;margin-top:4px;background:#18181b;border-radius:8px;padding:4px 0;min-width:140px;box-shadow:0 4px 16px rgba(0,0,0,.2);z-index:1}
.vlr-popover .vlr-menu button{display:block;width:100%;text-align:left;padding:6px 12px;background:none;border:none;color:#fff;font-size:12px;font-family:inherit;cursor:pointer;border-radius:0}
.vlr-popover .vlr-menu button:hover{background:rgba(255,255,255,.1);color:#fff}
.vlr-popover .vlr-compose{border-top:1px solid #e4e4e7;padding:8px 10px;display:flex;align-items:flex-end;gap:6px}
.vlr-popover .vlr-compose textarea{flex:1;border:none;background:transparent;padding:6px 0;font-family:inherit;font-size:12px;resize:none;outline:none;min-height:18px;max-height:80px;overflow-y:auto;line-height:1.4}
.vlr-send{width:28px;height:28px;border-radius:50%;border:none;display:flex;align-items:center;justify-content:center;cursor:pointer;flex-shrink:0;background:#d4d4d8;color:#fff;transition:background .15s;padding:0}
.vlr-send.active{background:#18181b}
.vlr-send:hover.active{background:#27272a}
.vlr-new-popover{position:fixed;z-index:2147483646;background:rgba(255,255,255,.92);-webkit-backdrop-filter:blur(20px) saturate(1.5);backdrop-filter:blur(20px) saturate(1.5);border:1px solid #e4e4e7;border-radius:20px;box-shadow:0 4px 24px rgba(0,0,0,.1);padding:6px 6px 6px 4px;min-width:220px;max-width:320px;font-family:Geist,Inter,system-ui,-apple-system,sans-serif;font-size:13px;display:flex;align-items:flex-end;gap:4px}
.vlr-new-popover textarea{flex:1;border:none;background:transparent;padding:7px 10px;font-family:inherit;font-size:13px;resize:none;outline:none;min-height:18px;max-height:120px;overflow-y:auto;line-height:1.4}
` + "`" + `;
document.head.appendChild(styleEl);

// Ensure body is positioned so absolute pins work
if(getComputedStyle(document.body).position==='static')document.body.style.position='relative';

// Fixed overlay for click capture in placing mode only
overlay=document.createElement('div');
overlay.id='vlr-overlay';
document.body.appendChild(overlay);

// ---- API helpers ----
function api(method,path,body){
  var opts={method:method,credentials:'same-origin',headers:{}};
  if(body){opts.headers['Content-Type']='application/json';opts.body=JSON.stringify(body)}
  return fetch('/__velori/api'+path,opts).then(function(r){return r.json()}).catch(function(){return {}});
}
function relTime(iso){
  var d=Date.now()-new Date(iso).getTime();
  if(d<60000)return 'just now';
  if(d<3600000)return Math.floor(d/60000)+'m ago';
  if(d<86400000)return Math.floor(d/3600000)+'h ago';
  return Math.floor(d/86400000)+'d ago';
}
function initial(name){return (name||'?').charAt(0).toUpperCase()}
function esc(s){var d=document.createElement('span');d.textContent=s;return d.innerHTML}

function clientToPin(clientX,clientY){
  var mt=parseFloat(getComputedStyle(document.body).marginTop)||0;
  var docW=document.documentElement.scrollWidth;
  var docH=document.body.scrollHeight;
  return{
    x:(clientX+window.scrollX)/docW*100,
    y:(clientY+window.scrollY-mt)/docH*100
  };
}

// ---- Pin rendering ----
// Pins are position:absolute in document.body, using percentage coordinates
// relative to the full document dimensions so they scroll with content.
function clearPins(){
  pins.forEach(function(p){if(p.parentNode)p.parentNode.removeChild(p)});
  pins=[];
}

function renderPins(comments){
  clearPins();
  cachedComments=comments;
  if(!commentMode)return;
  comments.forEach(function(c,i){
    var pin=document.createElement('div');
    pin.setAttribute('data-vlr-pin','');
    pin.textContent=String(i+1);
    pin.style.left=c.pinX+'%';
    pin.style.top=c.pinY+'%';
    var isOwn=c.userId===ctx.userId;
    if(isOwn)pin.classList.add('vlr-own');
    initPinDrag(pin,c,isOwn);
    document.body.appendChild(pin);
    pins.push(pin);
  });
}

// ---- Pin drag ----
var DRAG_THRESHOLD=4;
function initPinDrag(pin,c,isOwn){
  var startX,startY,dragging;
  function onDown(e){
    if(!commentMode)return;
    e.preventDefault();
    e.stopPropagation();
    var pt=e.touches?e.touches[0]:e;
    startX=pt.clientX;startY=pt.clientY;dragging=false;
    document.addEventListener('mousemove',onMove);
    document.addEventListener('mouseup',onUp);
    document.addEventListener('touchmove',onMove,{passive:false});
    document.addEventListener('touchend',onUp);
  }
  function onMove(e){
    var pt=e.touches?e.touches[0]:e;
    var dx=pt.clientX-startX,dy=pt.clientY-startY;
    if(!dragging&&Math.sqrt(dx*dx+dy*dy)<DRAG_THRESHOLD)return;
    if(!dragging){
      if(!isOwn){cleanup();return}
      dragging=true;
      closePopover();
      pin.classList.add('vlr-dragging');
    }
    e.preventDefault();
    var mc=clientToPin(pt.clientX,pt.clientY);
    pin.style.left=mc.x+'%';
    pin.style.top=mc.y+'%';
  }
  function onUp(e){
    cleanup();
    if(dragging){
      pin.classList.remove('vlr-dragging');
      var pt=e.changedTouches?e.changedTouches[0]:e;
      var fc=clientToPin(pt.clientX,pt.clientY);
      var px=Math.round(fc.x*10000)/10000;
      var py=Math.round(fc.y*10000)/10000;
      api('PATCH','/comments/'+c.id,{pinX:px,pinY:py}).then(function(){loadComments()});
    }else{
      openThread(c,pin);
    }
  }
  function cleanup(){
    document.removeEventListener('mousemove',onMove);
    document.removeEventListener('mouseup',onUp);
    document.removeEventListener('touchmove',onMove);
    document.removeEventListener('touchend',onUp);
  }
  pin.addEventListener('mousedown',onDown);
  pin.addEventListener('touchstart',onDown,{passive:false});
}

// ---- Load comments ----
function loadComments(cb){
  var qs='?project_id='+encodeURIComponent(ctx.projectId)+'&deploy_id='+encodeURIComponent(ctx.deployId)+'&page_path='+encodeURIComponent(currentPath);
  api('GET','/comments'+qs).then(function(data){
    var comments=data.comments||[];
    renderPins(comments);
    if(cb)cb(comments);
  });
}

// ---- Comment mode toggle ----
var commentBtn=sr.getElementById('velori-comment-btn');
function setCommentMode(on){
  commentMode=on;
  if(commentBtn)commentBtn.style.opacity=commentMode?'1':'.5';
  document.body.style.cursor=on?'crosshair':'';
  if(!commentMode){
    clearPins();
    closePopover();
    removeNewPopover();
  }else{
    renderPins(cachedComments);
  }
}
if(commentBtn){
  commentBtn.addEventListener('click',function(){setCommentMode(!commentMode)});
  commentBtn.style.opacity='.5';
}


// ---- New comment popover ----
var newPop=null;
var tempPin=null;

function removeNewPopover(){
  if(newPop){newPop.remove();newPop=null}
  if(tempPin){tempPin.remove();tempPin=null}
  document.body.style.overflow='';
}

function showNewPopover(px,py,screenX,screenY){
  removeNewPopover();
  tempPin=document.createElement('div');
  tempPin.setAttribute('data-vlr-pin','');
  tempPin.textContent='+';
  tempPin.style.left=px+'%';
  tempPin.style.top=py+'%';
  document.body.appendChild(tempPin);

  newPop=document.createElement('div');
  newPop.className='vlr-new-popover';
  var left=Math.min(screenX+12,window.innerWidth-340);
  var top=Math.min(screenY+12,window.innerHeight-60);
  newPop.style.left=left+'px';
  newPop.style.top=top+'px';

  var ta=document.createElement('textarea');
  ta.placeholder='Add a comment...';
  ta.rows=1;
  var sendBtn=document.createElement('button');
  sendBtn.className='vlr-send';
  sendBtn.innerHTML='<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="19" x2="12" y2="5"/><polyline points="5 12 12 5 19 12"/></svg>';
  sendBtn.onclick=function(){submitNew(px,py,ta.value)};
  ta.addEventListener('input',function(){
    ta.style.height='auto';
    ta.style.height=Math.min(ta.scrollHeight,120)+'px';
    sendBtn.classList.toggle('active',ta.value.trim().length>0);
  });
  ta.addEventListener('keydown',function(e){
    if(e.key==='Enter'&&(e.metaKey||e.ctrlKey)){submitNew(px,py,ta.value)}
    if(e.key==='Escape'){removeNewPopover()}
  });
  newPop.appendChild(ta);
  newPop.appendChild(sendBtn);

  document.body.style.overflow='hidden';
  document.body.appendChild(newPop);
  setTimeout(function(){ta.focus()},0);
}

function submitNew(px,py,body){
  body=(body||'').trim();
  if(!body)return;
  api('POST','/comments',{
    projectId:ctx.projectId,deployId:ctx.deployId,pagePath:currentPath,
    pinX:Math.round(px*10000)/10000,pinY:Math.round(py*10000)/10000,body:body
  }).then(function(data){
    if(data.error){alert(data.error);return}
    removeNewPopover();
    loadComments();
  });
}

// ---- Thread popover (anchored to pin) ----
function closePopover(){
  if(popover){popover.remove();popover=null}
}

function openThread(c,pinEl){
  closePopover();
  popover=document.createElement('div');
  popover.className='vlr-popover';

  var resolved=!!c.resolvedAt;

  // Header row: author info left, action icons right
  var header=document.createElement('div');
  header.className='vlr-pop-header';
  header.innerHTML='<div class="vlr-pop-header-left"><span class="vlr-avatar">'+initial(c.userName)+'</span><span class="vlr-author">'+esc(c.userName)+'</span><span class="vlr-time">'+relTime(c.createdAt)+'</span></div>';
  var actions=document.createElement('div');
  actions.className='vlr-toolbar';
  actions.style.position='relative';

  if(c.userId===ctx.userId){
    var moreBtn=document.createElement('button');
    moreBtn.title='More';
    moreBtn.innerHTML='<svg width="15" height="15" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="5" r="1.5"/><circle cx="12" cy="12" r="1.5"/><circle cx="12" cy="19" r="1.5"/></svg>';
    var menu=null;
    moreBtn.onclick=function(e){
      e.stopPropagation();
      if(menu){menu.remove();menu=null;return}
      menu=document.createElement('div');
      menu.className='vlr-menu';
      var del=document.createElement('button');
      del.textContent='Delete thread\u2026';
      del.onclick=function(ev){
        ev.stopPropagation();
        if(!confirm('Delete this comment and all replies?'))return;
        api('DELETE','/comments/'+c.id).then(function(){closePopover();loadComments()});
      };
      menu.appendChild(del);
      actions.appendChild(menu);
    };
    actions.appendChild(moreBtn);
  }

  var resolveBtn=document.createElement('button');
  resolveBtn.title=resolved?'Unresolve':'Resolve';
  if(resolved)resolveBtn.classList.add('vlr-resolve-active');
  resolveBtn.innerHTML='<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>';
  resolveBtn.onclick=function(){
    api('PATCH','/comments/'+c.id,{resolved:!resolved}).then(function(){closePopover();loadComments()});
  };
  actions.appendChild(resolveBtn);

  header.appendChild(actions);
  popover.appendChild(header);

  // Body content
  var body=document.createElement('div');
  body.className='vlr-pop-body';
  var bodyText=document.createElement('div');
  bodyText.className='vlr-body';
  bodyText.textContent=c.body;
  body.appendChild(bodyText);

  // Replies
  if(c.replies&&c.replies.length){
    var repliesDiv=document.createElement('div');
    repliesDiv.className='vlr-replies';
    c.replies.forEach(function(r){repliesDiv.appendChild(renderMsg(r,true))});
    body.appendChild(repliesDiv);
  }
  popover.appendChild(body);

  // Compose reply
  var compose=document.createElement('div');
  compose.className='vlr-compose';
  var ta=document.createElement('textarea');
  ta.placeholder='Reply...';
  ta.rows=1;
  var sendBtn=document.createElement('button');
  sendBtn.className='vlr-send';
  sendBtn.innerHTML='<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="19" x2="12" y2="5"/><polyline points="5 12 12 5 19 12"/></svg>';
  sendBtn.onclick=function(){submitReply(c,ta)};
  ta.addEventListener('input',function(){
    ta.style.height='auto';
    ta.style.height=Math.min(ta.scrollHeight,80)+'px';
    sendBtn.classList.toggle('active',ta.value.trim().length>0);
  });
  ta.addEventListener('keydown',function(e){
    if(e.key==='Enter'&&(e.metaKey||e.ctrlKey)){submitReply(c,ta)}
  });
  compose.appendChild(ta);
  compose.appendChild(sendBtn);
  popover.appendChild(compose);

  // Position anchored to pin
  document.body.appendChild(popover);
  positionPopover(popover,pinEl);
}

function positionPopover(pop,pinEl){
  var pr=pinEl.getBoundingClientRect();
  var pw=pop.offsetWidth;
  var ph=pop.offsetHeight;
  var left=pr.right+8+window.scrollX;
  var top=pr.top-8+window.scrollY;
  // Flip left if overflows right
  if(pr.right+8+pw>window.innerWidth){left=pr.left-pw-8+window.scrollX}
  // Clamp top
  if(top+ph>window.scrollY+window.innerHeight){top=window.scrollY+window.innerHeight-ph-8}
  if(top<window.scrollY+BAR_H+4){top=window.scrollY+BAR_H+4}
  pop.style.left=left+'px';
  pop.style.top=top+'px';
}

function renderMsg(c,isReply){
  var msg=document.createElement('div');
  msg.className=isReply?'vlr-reply-msg vlr-msg':'vlr-msg';
  var head=document.createElement('div');
  head.className='vlr-msg-head';
  head.innerHTML='<span class="vlr-avatar">'+initial(c.userName)+'</span><span class="vlr-author">'+esc(c.userName)+'</span><span class="vlr-time">'+relTime(c.createdAt)+'</span>';
  msg.appendChild(head);
  var bd=document.createElement('div');
  bd.className='vlr-body';
  bd.textContent=c.body;
  msg.appendChild(bd);
  return msg;
}

function submitReply(parent,ta){
  var body=(ta.value||'').trim();
  if(!body)return;
  api('POST','/comments',{
    projectId:ctx.projectId,deployId:ctx.deployId,pagePath:currentPath,
    body:body,parentId:parent.id
  }).then(function(data){
    if(data.error){alert(data.error);return}
    closePopover();
    loadComments();
  });
}

// ---- Single click handler for placing + closing ----
document.addEventListener('click',function(e){
  var isPin=e.target&&e.target.hasAttribute&&e.target.hasAttribute('data-vlr-pin');
  // Close popover on click outside
  if(popover&&!popover.contains(e.target)&&!isPin){closePopover();return}
  if(newPop&&!newPop.contains(e.target)&&e.target!==tempPin){removeNewPopover();return}
  // Place new comment in comment mode
  if(!commentMode||popover||newPop||isPin)return;
  if(e.clientY<BAR_H)return;
  e.preventDefault();
  var coords=clientToPin(e.clientX,e.clientY);
  var px=coords.x;
  var py=coords.y;
  showNewPopover(px,py,e.clientX,e.clientY);
});

// ---- SPA nav detection ----
var origPush=history.pushState;
var origReplace=history.replaceState;
function onNav(){
  var p=location.pathname.replace(/^\/~[^/]+\/[^/]+\/?/,'').replace(/^_v\/[^/]+\//,'').replace(/^\//,'');
  if(p!==currentPath){currentPath=p;loadComments()}
}
history.pushState=function(){origPush.apply(this,arguments);onNav()};
history.replaceState=function(){origReplace.apply(this,arguments);onNav()};
window.addEventListener('popstate',onNav);

// ---- Keyboard ----
document.addEventListener('keydown',function(e){
  if(e.key==='Escape'){
    if(newPop)removeNewPopover();
    else if(popover)closePopover();
    else if(commentMode)setCommentMode(false);
  }
  var tag=e.target&&e.target.tagName;
  if(tag==='INPUT'||tag==='TEXTAREA'||tag==='SELECT'||e.target.isContentEditable)return;
  if(e.key==='c'&&!e.metaKey&&!e.ctrlKey&&!e.altKey){
    setCommentMode(!commentMode);
  }
});

// ---- Init ----
loadComments();
})();`

var commentsJSBytes = []byte(commentsJS)

func (app *application) commentsJSHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(commentsJSBytes)
}
