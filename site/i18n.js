/* Offline EN/KO localization. Only exact catalog entries or anchored message
   templates are translated. User content, code and model/tool IDs opt out. */
(function (root) {
  "use strict";
  const catalog = root.TraceFrugalKorean || {strings:{},patterns:[]};
  const escape = text => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const patterns = catalog.patterns.map(([source,target]) => {
    const names=[];
    const parts=source.split(/(\{\{\w+\}\})/g);
    const expression=parts.map(part=>{
      if(/^\{\{\w+\}\}$/.test(part)){names.push(part.slice(2,-2));return "([\\s\\S]*?)";}
      return escape(part);
    }).join("");
    return {match:new RegExp("^"+expression+"$"),names,target};
  });
  let requested, saved;
  try { requested=new URL(root.location.href).searchParams.get("lang"); } catch {}
  try { saved=root.localStorage.getItem("tracefrugal.language"); } catch {}
  let language=["en","ko"].includes(requested)?requested:["en","ko"].includes(saved)?saved:(root.navigator?.language||"").startsWith("ko")?"ko":"en";
  function t(text, values={}) {
    let result=text;
    if(language==="ko"){
      if(Object.hasOwn(catalog.strings,text))result=catalog.strings[text];
      else for(const p of patterns){
        const match=p.match.exec(text);
        if(match){const found=Object.fromEntries(p.names.map((name,i)=>[name,match[i+1]]));result=p.target.replace(/\{\{(\w+)\}\}/g,(_,key)=>found[key]??"");break;}
      }
    }
    return result.replace(/\{\{(\w+)\}\}/g,(_,key)=>values[key]??"{{"+key+"}}");
  }
  const originals=new WeakMap();
  function translated(owner,key,current,set){
    if(!current.trim())return;
    let entries=originals.get(owner);
    if(!entries){entries={};originals.set(owner,entries);}
    let record=entries[key];
    if(!record || current!==record.last)record=entries[key]={source:current,last:current};
    const leading=record.source.match(/^\s*/)[0], trailing=record.source.match(/\s*$/)[0];
    const next=leading+t(record.source.trim())+trailing;
    if(next!==current)set(next);
    record.last=next;
  }
  function excluded(node){
    return node.parentElement?.closest("script,style,pre,code,textarea,[data-no-i18n],.tool-name,.model-name");
  }
  function localize(node=root.document?.documentElement){
    if(!node)return;
    if(node.nodeType===3){
      if(!excluded(node))translated(node,"text",node.nodeValue,value=>{node.nodeValue=value;});
      return;
    }
    if(node.nodeType!==1 || node.closest("script,style,pre,code,textarea,[data-no-i18n],.tool-name,.model-name"))return;
    for(const key of ["aria-label","title","placeholder","alt"]){
      if(node.hasAttribute(key))translated(node,key,node.getAttribute(key),value=>node.setAttribute(key,value));
    }
    for(const child of Array.from(node.childNodes))localize(child);
  }
  function setLanguage(value){
    if(!["en","ko"].includes(value))return;
    language=value;
    try{root.localStorage.setItem("tracefrugal.language",value);}catch{}
    root.document.documentElement.lang=value;
    root.document.querySelectorAll("[data-language]").forEach(select=>{select.value=value;});
    root.dispatchEvent(new Event("tracefrugal:language"));
    localize();
  }
  root.I18n={t,localize,setLanguage,get language(){return language;},get locale(){return language==="ko"?"ko-KR":"en-US";}};
  if(!root.document)return;
  root.document.documentElement.lang=language;
  root.document.querySelectorAll("[data-language]").forEach(select=>{
    select.value=language;
    select.addEventListener("change",event=>setLanguage(event.target.value));
  });
  localize();
  if(root.MutationObserver){
    // A single pass per mutation batch; idempotent updates avoid observer loops.
    new MutationObserver(records=>{
      for(const record of records){
        if(record.type==="characterData"||record.type==="attributes")localize(record.target);
        else for(const node of record.addedNodes)localize(node);
      }
    }).observe(root.document.documentElement,{subtree:true,childList:true,characterData:true,attributes:true,attributeFilter:["aria-label","title","placeholder","alt"]});
  }
})(globalThis);
