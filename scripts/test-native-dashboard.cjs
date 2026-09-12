const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const elements = new Map();
function element(id) {
  if(!elements.has(id))elements.set(id,{
    textContent:id==="bootstrap"?'{"mode":"native-demo"}':"",innerHTML:"",value:"",hidden:false,
    classList:{toggle(){}},addEventListener(){},setAttribute(){},removeAttribute(){},
    showModal(){this.open=true},close(){this.open=false}
  });
  return elements.get(id);
}
const context=vm.createContext({document:{getElementById:element,querySelectorAll:()=>[],addEventListener(){}},setInterval(){},URLSearchParams,console});
vm.runInContext(fs.readFileSync("internal/webui/claude.js","utf8"),context);
const run=code=>vm.runInContext(code,context);
async function test(){
  await run("refresh()");
  assert.equal(run("state.sources.length"),3);
  assert.equal(run("state.summary.requests"),run("state.sources.reduce((n,s)=>n+s.summary.requests,0)"));
  assert.equal(run("state.summary.tokens.output"),run("state.sources.reduce((n,s)=>n+s.summary.tokens.output,0)"));
  assert.ok(element("sources").innerHTML.includes("sample-codex"));
  assert.ok(element("quality").innerHTML.includes("already included in output"));
  assert.ok(element("packing-history").innerHTML.includes("Hour 24"));
  assert.ok(element("packing-panel").innerHTML.includes("not measured token or dollar savings"));
  run('sourceID="sample-codex"');
  await run("refresh()");
  assert.equal(run("state.recipes.length"),0);
  assert.equal(run("state.summary.unpriced"),run("state.summary.requests"));
  assert.equal(run("state.diagnostics.human_turns"),0);
  assert.ok(element("trial-card").innerHTML.includes("read-only"));
  run("unit='cost';renderChart()");
  assert.ok(element("chart").innerHTML.includes("not zero cost"));
  run('sourceID="sample-claude"');
  await run("refresh()");
  assert.equal(run("state.recipes.length"),3);
  run('recipeID="mcp"');
  await run('change("try","",4)');
  assert.equal(run("state.changes.length"),1);
  run('sourceID="sample-codex";state=undefined');
  await run("refresh()");
  assert.equal(run("state.changes.length"),0);
  run('sourceID="sample-claude";state=undefined');
  await run("refresh()");
  assert.equal(run("state.changes.length"),1);
  element("rating-pack").value="2";
  await run('packingAction("pack-rate")');
  assert.ok(element("packing-history").innerHTML.includes("Satisfaction fell"));
  await run('packingAction("pack-stop")');
  assert.ok(element("packing-history").innerHTML.includes("Stopped"));
  run('state.sources[0].name="<img src=x onerror=bad>";renderSources()');
  assert.ok(!element("sources").innerHTML.includes("<img"));
  console.log("Native dashboard: source sums/filtering, price gaps, reasoning, scoped history, packing graph, ratings, stop and escaping passed.");
}
test().catch(e=>{console.error(e);process.exitCode=1});
