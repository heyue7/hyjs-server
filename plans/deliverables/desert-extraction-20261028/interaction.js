'use strict';
const DATA=JSON.parse(document.getElementById('designData').textContent);
const $=id=>document.getElementById(id),fmt=(n,d=0)=>Number(n).toLocaleString('zh-CN',{minimumFractionDigits:d,maximumFractionDigits:d});
const types={mob:['小怪','#e6bb68'],boss:['Boss','#ed7668'],mine:['矿','#b8c6d9'],fish:['鱼','#78d7e2'],exit:['接应','#90d0aa'],safe:['安全区','#e9ead7']};
Object.entries(types).forEach(([k,v])=>{let l=document.createElement('label');l.innerHTML='<input type="checkbox" checked data-layer="'+k+'" aria-label="显示'+v[0]+'"><i style="background:'+v[1]+'"></i>'+v[0];$('mapTools').appendChild(l)});
const ns='http://www.w3.org/2000/svg';
DATA.mapMarkers.forEach(p=>{
 const g=document.createElementNS(ns,'g');g.setAttribute('class','marker');g.dataset.kind=p.kind;g.setAttribute('tabindex','0');g.setAttribute('role','button');g.setAttribute('aria-label',p.id+' '+p.title);g.setAttribute('transform','translate('+p.x+','+p.y+')');
 const c=document.createElementNS(ns,'circle');c.setAttribute('r',p.kind==='boss'?10:8);c.setAttribute('fill',types[p.kind][1]);c.setAttribute('stroke','#132e3d');c.setAttribute('stroke-width','1.5');
 const t=document.createElementNS(ns,'text');t.setAttribute('text-anchor','middle');t.setAttribute('dy','3');t.setAttribute('font-size','6.5');t.setAttribute('font-weight','800');t.setAttribute('fill','#112a36');t.textContent=p.id;
 const title=document.createElementNS(ns,'title');title.textContent=p.id+' · '+p.title;g.append(c,t,title);
 const show=()=>{const box=$('pointDetail');box.innerHTML='';let h=document.createElement('h3');h.textContent=p.id+' · '+p.title;let b=document.createElement('p');b.textContent=p.text;let sm=document.createElement('small');sm.textContent='图像坐标 ('+p.x+', '+p.y+') · 候选区域标记，世界坐标待核';box.append(h,b,sm)};
 g.addEventListener('click',show);g.addEventListener('keydown',e=>{if(e.key==='Enter'||e.key===' '){e.preventDefault();show()}});$('markers').appendChild(g);
});
function layers(){document.querySelectorAll('[data-layer]').forEach(c=>document.querySelectorAll('[data-kind="'+c.dataset.layer+'"]').forEach(x=>{x.style.display=c.checked?'':'none';x.setAttribute('tabindex',c.checked?'0':'-1')}));$('mapCount').textContent='当前展示 '+[...document.querySelectorAll('.marker')].filter(e=>e.style.display!=='none').length+' 个候选标记。M类24个独立怪位；O类18矿；F类12鱼；B类唯一Boss。'}
$('mapTools').addEventListener('change',layers);layers();
function rng(seed){let a=seed>>>0;return()=>{a+=0x6D2B79F5;let t=a;t=Math.imul(t^t>>>15,t|1);t^=t+Math.imul(t^t>>>7,t|61);return((t^t>>>14)>>>0)/4294967296}}
function draw(r,w){let x=r(),s=0;for(let i=0;i<w.length;i++){s+=w[i];if(x<s)return i}return w.length-1}
let demoSeed=20261028;
DATA.qualityNames.forEach((n,i)=>{let l=document.createElement('label');l.innerHTML=n+'芯片<input type="number" min="0" max="1000" step="1" value="'+[4,5,5,4,1,1][i]+'" aria-label="'+n+'芯片数量" id="q'+i+'">';$('secureInputs').appendChild(l)});
function secureDemo(randomize=false){const r=rng(randomize?++demoSeed:demoSeed);let counts=DATA.qualityNames.map((_,i)=>Math.min(1000,Math.max(0,Math.floor(Number($('q'+i).value)||0))));let secure=counts.map(()=>0),left=3;for(let i=5;i>=0;i--){secure[i]=Math.min(left,counts[i]);left-=secure[i]}let pool=[];counts.forEach((n,i)=>{for(let j=0;j<n-secure[i];j++)pool.push(i)});const k=Math.min(Math.floor(counts.reduce((a,b)=>a+b,0)/2),pool.length),lost=counts.map(()=>0);for(let j=0;j<k;j++){let ix=j+Math.floor(r()*(pool.length-j));[pool[j],pool[ix]]=[pool[ix],pool[j]];lost[pool[j]]++}const desc=a=>a.map((n,i)=>n?DATA.qualityNames[i]+'×'+n:'').filter(Boolean).reverse().join('、')||'无',money=a=>a.reduce((s,n,i)=>s+n*DATA.values[i],0);
 $('secureResult').innerHTML='<p><b>保护：</b>'+desc(secure)+'<br><b>未保护：</b>'+pool.length+'枚 → 本次扣除 '+k+'枚<br><b>随机损失：</b>'+desc(lost)+'，价值'+fmt(money(lost))+'信用<br><b>死亡后保留：</b>'+desc(counts.map((n,i)=>n-lost[i]))+'，价值'+fmt(money(counts)-money(lost))+'信用</p>';
 window.lastSecureDemo={counts,secure,lost,k};
}
$('deathDemo').addEventListener('click',()=>secureDemo(true));$('secureInputs').addEventListener('input',()=>secureDemo());secureDemo();
function calculate(){const minutes=+$('minutes').value,people=+$('people').value,death=+$('death').value/100,congestion=Math.min(1,15/people),mobs=Math.floor(minutes/3*congestion),gathers=Math.min(50,Math.floor(minutes*16/60*congestion)),bosses=$('noBoss').checked?0:Math.min(3,Math.floor(minutes/60)),trips=Math.ceil(minutes/20),samples=4000,r=rng(20261028+minutes+people*1000+Math.round(death*10000)+bosses),nets=[];let lostSum=0;const gp=DATA.tierMix.reduce((s,p,i)=>s+p*DATA.gatherTrigger[i],0),mp=DATA.mobMix.reduce((s,p,i)=>s+p*DATA.mobTriggers[i],0),gross=(mobs*mp+gathers*gp)*19.5+bosses*60;
 for(let n=0;n<samples;n++){const bags=Array.from({length:trips},()=>[]);for(let i=0;i<mobs;i++){const tier=draw(r,DATA.mobMix);if(r()<DATA.mobTriggers[tier])bags[Math.min(trips-1,Math.floor((i+.5)*minutes/mobs/20))].push(DATA.values[draw(r,DATA.qualityProb)])}for(let i=0;i<gathers;i++){let tier=draw(r,DATA.tierMix);if(r()<DATA.gatherTrigger[tier])bags[Math.min(trips-1,Math.floor((i+.5)*minutes/gathers/20))].push(DATA.values[draw(r,DATA.qualityProb)])}for(let i=0;i<bosses;i++){const ix=Math.min(trips-1,(i+1)*3-1);bags[ix].push(30,30)}let net=0;for(const bag of bags){bag.sort((a,b)=>b-a);let total=bag.reduce((s,n)=>s+n,0),pool=bag.slice(3);if(pool.length&&r()<death){let k=Math.min(Math.floor(bag.length/2),pool.length);for(let j=0;j<k;j++){const ix=j+Math.floor(r()*(pool.length-j));[pool[j],pool[ix]]=[pool[ix],pool[j]];total-=pool[j];lostSum+=pool[j]}}net+=total}nets.push(net)}
 nets.sort((a,b)=>a-b);const mean=gross-lostSum/samples,q=[nets[Math.floor(samples*.1)],nets[Math.floor(samples*.5)],nets[Math.floor(samples*.9)]];
 $('minutesVal').textContent=minutes+'分钟';$('peopleVal').textContent=people+'人';$('deathVal').textContent=Math.round(death*100)+'%';
 $('actions').textContent='每天 '+mobs+' 场小怪、'+gathers+' 次采集（'+gathers*4+'疲劳）、'+bosses+' 次有效沙漠Boss、'+trips+' 次出发。'+(people>20?'此人数超过建议上线硬容量，仅作压力情景。':'');
 $('gross').textContent=fmt(gross,1);$('net').textContent=fmt(mean);$('median').textContent=fmt(q[1]);$('line').textContent=fmt(gross*people);
 const vals=[...q,mean],max=Math.max(...vals,1);$('chart').innerHTML=vals.map((v,i)=>'<div class="bar" style="height:'+Math.max(2,v/max*125)+'px;background:'+(i===3?'#c29952':'#799883')+'"><span>'+fmt(v)+'</span><label>'+['P10','中位数','P90','均值'][i]+'</label></div>').join('');
 $('simStatus').textContent='4000样本日；均值=解析发行−模拟损失，分位为样本。个人平均损失'+fmt(lostSum/samples,1)+'信用。整线发行期望'+fmt(gross*people)+'；若损失金额30%来自PvE，约'+fmt((gross-lostSum/samples*.3)*people)+'理论信用等值（封闭人群且未重演PvP转入后的再次损失），未扣消费／留存，不含排名尾刀。';
 window.lastSimulation={minutes,people,death,mobs,gathers,bosses,trips,gross,mean,q,line:gross*people};
}
let timer;document.querySelectorAll('.controls input,#noBoss').forEach(e=>e.addEventListener('input',()=>{clearTimeout(timer);timer=setTimeout(calculate,70)}));calculate();
const r60=DATA.simulations.find(r=>r.minutes===60),r120=DATA.simulations.find(r=>r.minutes===120);
$('lineExample').innerHTML='<b>整线示例：</b>15名角色各玩2小时，共30玩家小时，每人有效参与2次Boss且贡献达3%。基础发行 '+fmt(r120.gross*15)+' 信用等值，加两场完整排名尾刀期望714，全体毛发行 '+fmt(r120.gross*15+714)+'。若不计额外奖的后续死亡，且基础损失金额30%来自PvE，基础理论留存 '+fmt((r120.gross-r120.meanLoss*.3)*15)+'；PvP转入后再次死亡未重演。一天3批同样15人（45名不同角色、90玩家小时、6场完整Boss）的毛发行约 '+fmt(r120.gross*45+2142)+'。人数、有效资格与场次数必须同时满足，不能据同线15人直接推出全服日产。';
$('budgetText').innerHTML='<b>上调后的周预算：</b>1小时型周均约'+fmt(r60.mean*7)+'信用，可预留超频7份910＋普通材料270（解放石5份×30、饰品精炼石2份×60）＋刷新10次×15=150，总计1330，余约'+fmt(r60.mean*7-1330)+'。2小时型周均约'+fmt(r120.mean*7)+'，预留超频910＋普通材料540＋刷新30次×15=450＋神圣片一份840，总计2740，余约'+fmt(r120.mean*7-2740)+'。这些是购买预算，不能保证见货；多数只拿蓝1的玩家每次Boss会少30毛收入。';
$('budgetDelta').innerHTML='<b>变化与约束：</b>个人均值约'+fmt(r60.mean)+'／'+fmt(r120.mean)+'信用（1／2小时）。相对旧条件预算100／250为'+(r60.mean/100).toFixed(2)+'／'+(r120.mean/250).toFixed(2)+'倍。商品上调约40%—50%与新增Boss、分级怪收入一起评估，未降低任何旧售价。刷满货架需1575信用/周，1小时型难以同时满额刷新和买满超频；不承诺所有消费都能覆盖。';
