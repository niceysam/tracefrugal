const assert=require("node:assert/strict"),fs=require("node:fs"),vm=require("node:vm");
class Element{
  constructor(tag="div",attrs={},children=[]){this.nodeType=1;this.tag=tag;this.attrs=attrs;this.childNodes=children;for(const c of children)c.parentElement=this;}
  closest(){return this.attrs["data-no-i18n"]!==undefined||["code","pre","script","style","textarea"].includes(this.tag)?this:this.parentElement?.closest();}
  hasAttribute(k){return k in this.attrs} getAttribute(k){return this.attrs[k]} setAttribute(k,v){this.attrs[k]=v}
}
const text=s=>({nodeType:3,nodeValue:s});
function setup({href="https://example.test",saved,nav="en-US",blocked=false}={}){
  const label=text("  New input  "), space=text(" "), skip=text("New input");
  const el=new Element("html",{},[new Element("span",{title:"New input"},[label,space]),new Element("span",{"data-no-i18n":""},[new Element("span",{},[skip])])]);
  const values=new Map(saved?[["tracefrugal.language",saved]]:[]);
  const root={URL,Event:class{},location:{href},navigator:{language:nav},localStorage:{getItem:k=>{if(blocked)throw Error();return values.get(k)},setItem:(k,v)=>{if(blocked)throw Error();values.set(k,v)}},document:{documentElement:el,querySelectorAll:()=>[]},dispatchEvent(){}};
  const ctx=vm.createContext(root);
  for(const name of ["i18n-ko.js","i18n.js","economics.js","inspector.js"])vm.runInContext(fs.readFileSync("internal/webui/"+name,"utf8"),ctx);
  return {ctx,el,label,space,skip,values};
}
const a=setup({href:"https://example.test/?lang=ko",saved:"en"});
assert.equal(a.ctx.I18n.language,"ko");
assert.equal(a.label.nodeValue,"  새 입력  ");
assert.equal(a.space.nodeValue," ");
assert.equal(a.skip.nodeValue,"New input");
a.ctx.I18n.localize(a.skip.parentElement);
assert.equal(a.skip.nodeValue,"New input","attribute mutations must respect opt-out ancestors");
a.ctx.I18n.setLanguage("en");assert.equal(a.label.nodeValue,"  New input  ");
a.ctx.I18n.setLanguage("ko");assert.equal(a.label.nodeValue,"  새 입력  ");
a.label.nodeValue="Output";a.ctx.I18n.localize(a.label);assert.equal(a.label.nodeValue,"출력");
a.ctx.I18n.setLanguage("en");assert.equal(a.label.nodeValue,"Output","dynamic text must not restore stale originals");
assert.equal(setup({href:"https://x.test?lang=ko",blocked:true}).ctx.I18n.language,"ko");
assert.equal(setup({saved:"en",nav:"ko-KR"}).ctx.I18n.language,"en");
assert.equal(setup({nav:"ko-KR"}).ctx.I18n.language,"ko");
const catalog=a.ctx.TraceFrugalKorean;
assert.ok(Object.keys(catalog.strings).length>300);
assert.ok(!JSON.stringify(catalog).includes("\uFFFD"),"replacement characters in translations");
for(const [english,korean] of catalog.patterns){
  assert.deepEqual([...english.matchAll(/\{\{(\w+)\}\}/g)].map(m=>m[1]).sort(),[...korean.matchAll(/\{\{(\w+)\}\}/g)].map(m=>m[1]).sort(),"template variables must be preserved");
}
const inspect=a.ctx.TraceInspector;
assert.notEqual(inspect.shortID({id:"claude:abcdefghij123"}),inspect.shortID({id:"claude:1234567890abc"}));
inspect.saveName("claude:abcdefghij","<img src=x onerror=bad>");
const tokens={input:100,cached_input:200,cache_write:0,cache_write_1h:0,output:10};
const s={id:"claude:abcdefghij",project:"private-project",start:"2026-09-12T00:00:00Z",end:"2026-09-12T00:01:00Z",models:["model"],requests:1,tokens,known_usd:0,unpriced:1,prices:[{model:"model",requests:1,tokens,unpriced:1,reason:"native_codex_unpriced"}]};
const report={sessions:[s],price_label:"dated rates"};
assert.ok(inspect.html(s,report,false,"").includes("&lt;img"));
assert.ok(!inspect.html(s,report,false,"").includes("<img"));
assert.ok(!inspect.html(s,report,true,"").includes("private-project"));
assert.ok(!inspect.html(s,report,true,"").includes("onerror"));
const exported=JSON.stringify(inspect.exportData({...s,timeline:[{text:"PRIVATE_SENTINEL"}]},report));
assert.ok(!exported.includes("PRIVATE_SENTINEL")&&!exported.includes("private-project")&&!exported.includes("onerror"));
assert.ok(inspect.html(s,report,false,"").includes("Native Codex pricing is not established."));
assert.equal(setup({blocked:true}).ctx.TraceInspector.saveName("id","name"),false);
console.log("Localization: URL/storage/locale precedence, reversible and dynamic text, whitespace, opt-out ancestry, catalog integrity; inspector identity, escaped aliases, privacy and unpriced reasons passed.");
