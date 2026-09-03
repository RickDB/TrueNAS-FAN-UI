const $ = s => document.querySelector(s);
const fansEl = $('#fans');
const tempsEl = $('#temps');
let last = null;
let refreshTimer = null;
let toastTimer = null;

function esc(v='') { return String(v).replace(/[&<>'"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[c])); }
function showToast(msg, error=false) {
  const t=$('#toast'); t.textContent=msg; t.className='toast show'+(error?' error':'');
  clearTimeout(toastTimer); toastTimer=setTimeout(()=>t.className='toast',3200);
}
async function api(path, options={}) {
  const res=await fetch(path,{headers:{'Content-Type':'application/json'},...options});
  let data={}; try { data=await res.json(); } catch {}
  if(!res.ok) throw new Error(data.error || `${res.status} ${res.statusText}`);
  return data;
}
function modeLabel(v) {
  if(v===0) return 'disabled / full';
  if(v===1) return 'manual';
  if(v===2) return 'automatic';
  if(v==null) return 'unknown';
  return `mode ${v}`;
}
function fanCard(f, minPercent) {
  const rpm=f.rpm ?? null;
  const pct=f.percent ?? null;
  const mode=modeLabel(f.pwm_mode);
  const slider=pct ?? minPercent;
  const aliasesChanged = document.activeElement?.dataset?.fanName === f.id;
  return `<article class="fan-card ${rpm===0?'offline':''}" data-fan-card="${f.id}">
    <div class="fan-top">
      <div><div class="fan-name" data-name-label="${f.id}">${esc(f.name)}</div><div class="fan-meta">${esc(f.chip)} · channel ${f.index}<br>${esc(f.device)}</div></div>
      <div class="rpm">${rpm==null?'—':rpm}<small>RPM</small></div>
    </div>
    <div class="badge-row">
      <span class="badge ${f.writable?'good':''}">${f.writable?'PWM writable':'monitor only'}</span>
      ${pct==null?'':`<span class="badge">${pct}% · ${f.pwm}/255</span>`}
      ${f.pwm_mode==null?'':`<span class="badge ${f.pwm_mode===1?'warn':''}">${esc(mode)}</span>`}
      ${rpm===0?'<span class="badge warn">0 RPM</span>':''}
    </div>
    <div class="name-row">
      <input data-fan-name="${f.id}" value="${esc(f.name)}" placeholder="${esc(f.default_name)}" title="Clear and save to restore the hardware label">
      <button class="save-name" data-save-name="${f.id}">Save</button>
    </div>
    ${f.writable?`<div class="control-block">
      <div class="control-head"><span>Manual PWM</span><strong><span data-slider-value="${f.id}">${slider}</span>%</strong></div>
      <div class="slider-row"><input type="range" min="${minPercent}" max="100" step="1" value="${slider}" data-pwm-slider="${f.id}"><button class="apply-pwm" data-apply-pwm="${f.id}">Apply</button></div>
      <div class="control-actions"><span class="fan-meta">Safety floor ${minPercent}%</span>${f.can_restore_mode?`<button class="ghost restore-one" data-restore="${f.id}">Restore startup mode</button>`:''}</div>
    </div>`:''}
  </article>`;
}
function render(data) {
  last=data;
  const fans=data.fans || [], temps=data.temperatures || [];
  $('#fanCount').textContent=fans.length;
  $('#writableCount').textContent=`${fans.filter(f=>f.writable).length} controllable`;
  $('#minPercent').textContent=data.min_percent;
  $('#activeProfile').textContent=data.last_profile || 'Manual';
  const rpms=fans.map(f=>f.rpm).filter(v=>Number.isFinite(v));
  $('#avgRpm').textContent=rpms.length?Math.round(rpms.reduce((a,b)=>a+b,0)/rpms.length):'—';
  const hottest=temps.slice().sort((a,b)=>b.celsius-a.celsius)[0];
  $('#maxTemp').textContent=hottest?hottest.celsius.toFixed(1):'—';
  $('#maxTempLabel').textContent=hottest?`${hottest.label} · ${hottest.chip}`:'No temperature sensors';
  $('#updatedAt').textContent=`Updated ${new Date(data.now).toLocaleTimeString()}`;

  const focusedName=document.activeElement?.dataset?.fanName;
  const oldValue=focusedName?document.activeElement.value:null;
  fansEl.innerHTML=fans.length?fans.map(f=>fanCard(f,data.min_percent)).join(''):'<div class="empty">No fan or PWM channels exposed through hwmon.</div>';
  if(focusedName){ const el=document.querySelector(`[data-fan-name="${focusedName}"]`); if(el){el.value=oldValue;el.focus();el.setSelectionRange(el.value.length,el.value.length);} }

  tempsEl.innerHTML=temps.length?temps.map(t=>`<article class="temp-card ${t.celsius>=70?'hot':''}"><div class="temp-name">${esc(t.label)}</div><div class="temp-value">${t.celsius.toFixed(1)}°</div><div class="temp-chip">${esc(t.chip)}</div></article>`).join(''):'<div class="empty">No hwmon temperatures found.</div>';

  const profiles=Object.entries(data.profiles||{}).sort((a,b)=>a[1]-b[1]);
  $('#profiles').innerHTML=profiles.map(([n,p])=>`<button class="profile-btn ${data.last_profile===n?'active':''}" data-profile="${esc(n)}"><strong>${esc(n)}</strong><span>${p}% PWM</span></button>`).join('');
}
async function refresh(silent=false){
  try{
    const data=await api('/api/status'); render(data); $('#statusDot').className='dot ok'; $('#statusText').textContent='Live';
  }catch(e){ $('#statusDot').className='dot bad'; $('#statusText').textContent='Disconnected'; if(!silent)showToast(e.message,true); }
}

document.addEventListener('input', e=>{
  if(e.target.matches('[data-pwm-slider]')) document.querySelector(`[data-slider-value="${e.target.dataset.pwmSlider}"]`).textContent=e.target.value;
});
document.addEventListener('click', async e=>{
  const save=e.target.closest('[data-save-name]');
  const apply=e.target.closest('[data-apply-pwm]');
  const profile=e.target.closest('[data-profile]');
  const restore=e.target.closest('[data-restore]');
  try{
    if(save){ const id=save.dataset.saveName, input=document.querySelector(`[data-fan-name="${id}"]`); await api(`/api/fans/${id}/name`,{method:'POST',body:JSON.stringify({name:input.value})}); showToast('Fan name saved'); await refresh(true); }
    if(apply){ const id=apply.dataset.applyPwm, slider=document.querySelector(`[data-pwm-slider="${id}"]`); await api(`/api/fans/${id}/pwm`,{method:'POST',body:JSON.stringify({percent:Number(slider.value)})}); showToast(`PWM set to ${slider.value}%`); await refresh(true); }
    if(profile){ await api('/api/profile',{method:'POST',body:JSON.stringify({profile:profile.dataset.profile})}); showToast(`${profile.dataset.profile} profile applied`); await refresh(true); }
    if(restore){ await api(`/api/fans/${restore.dataset.restore}/restore`,{method:'POST',body:'{}'}); showToast('Startup PWM mode restored'); await refresh(true); }
  }catch(err){ showToast(err.message,true); }
});
$('#restoreAll').addEventListener('click',async()=>{ try{ await api('/api/restore',{method:'POST',body:'{}'}); showToast('Startup PWM modes restored'); await refresh(true); }catch(e){showToast(e.message,true);} });
$('#refreshBtn').addEventListener('click',()=>refresh());
refresh(); refreshTimer=setInterval(()=>refresh(true),2000);
