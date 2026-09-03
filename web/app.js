const $ = s => document.querySelector(s);
const fansEl = $('#fans');
const tempsEl = $('#temps');
let last = null;
let refreshTimer = null;
let toastTimer = null;
const pendingPWM = new Map();

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
  const slider = pendingPWM.has(f.id)
    ? pendingPWM.get(f.id)
    : (f.custom_percent ?? pct ?? minPercent);
  return `<article class="fan-card ${rpm===0?'offline':''}" data-fan-card="${f.id}">
    <div class="fan-top">
      <div><div class="fan-name" data-name-label="${f.id}">${esc(f.name)}</div><div class="fan-meta">${esc(f.chip)} · channel ${f.index}<br>${esc(f.device)}</div></div>
      <div class="rpm">${rpm==null?'—':rpm}<small>RPM</small></div>
    </div>
    <div class="badge-row">
      <span class="badge ${f.writable?'good':''}">${f.writable?'PWM writable':'monitor only'}</span>
      ${pct==null?'':`<span class="badge">${pct}% · ${f.pwm}/255</span>`}
      ${f.pwm_mode==null?'':`<span class="badge ${f.pwm_mode===1?'warn':''}">${esc(mode)}</span>`}
      ${f.custom_percent==null?'':`<span class="badge good">saved ${f.custom_percent}%</span>`}
      ${f.restore_on_startup?'<span class="badge good">startup restore</span>':''}
      ${rpm===0?'<span class="badge warn">0 RPM</span>':''}
    </div>
    <div class="name-row">
      <input data-fan-name="${f.id}" value="${esc(f.name)}" placeholder="${esc(f.default_name)}" title="Clear and save to restore the hardware label">
      <button class="save-name" data-save-name="${f.id}">Save</button>
    </div>
    ${f.writable?`<div class="control-block">
      <div class="control-head"><span>Custom fan profile</span><strong><span data-slider-value="${f.id}">${slider}</span>%</strong></div>
      <div class="slider-row"><input type="range" min="${minPercent}" max="100" step="1" value="${slider}" data-pwm-slider="${f.id}"><button class="apply-pwm" data-save-profile="${f.id}">Apply & save</button></div>
      <label class="startup-toggle">
        <input type="checkbox" data-startup-profile="${f.id}" ${f.restore_on_startup?'checked':''}>
        <span>Restore this custom profile on app startup</span>
      </label>
      <div class="control-actions">
        <span class="fan-meta">Safety floor ${minPercent}%</span>
        <div class="fan-action-group">
          ${f.custom_percent==null?'':`<button class="ghost clear-profile" data-clear-profile="${f.id}">Clear saved profile</button>`}
          ${f.can_restore_mode?`<button class="ghost restore-one" data-restore="${f.id}">Restore startup mode</button>`:''}
        </div>
      </div>
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

  const sortedTemps=temps.slice().sort((a,b)=>b.celsius-a.celsius);
  tempsEl.innerHTML=sortedTemps.length?sortedTemps.map(t=>`<article class="temp-card ${t.celsius>=70?'hot':''}"><div class="temp-name">${esc(t.label)}</div><div class="temp-value">${t.celsius.toFixed(1)}°</div><div class="temp-chip">${esc(t.chip)}</div></article>`).join(''):'<div class="empty">No hwmon temperatures found.</div>';

  const profiles=Object.entries(data.profiles||{}).sort((a,b)=>a[1]-b[1]);
  $('#profiles').innerHTML=profiles.map(([n,p])=>`<button class="profile-btn ${data.last_profile===n?'active':''}" data-profile="${esc(n)}"><strong>${esc(n)}</strong><span>${p}% PWM</span></button>`).join('');
}
async function refresh(silent=false){
  try{
    const data=await api('/api/status'); render(data); $('#statusDot').className='dot ok'; $('#statusText').textContent='Live';
  }catch(e){ $('#statusDot').className='dot bad'; $('#statusText').textContent='Disconnected'; if(!silent)showToast(e.message,true); }
}

document.addEventListener('input', e=>{
  if(!e.target.matches('[data-pwm-slider]')) return;

  const id=e.target.dataset.pwmSlider;
  const value=Number(e.target.value);
  pendingPWM.set(id,value);

  const label=document.querySelector(`[data-slider-value="${id}"]`);
  if(label) label.textContent=value;
});
document.addEventListener('click', async e=>{
  const save=e.target.closest('[data-save-name]');
  const saveProfile=e.target.closest('[data-save-profile]');
  const clearProfile=e.target.closest('[data-clear-profile]');
  const profile=e.target.closest('[data-profile]');
  const restore=e.target.closest('[data-restore]');
  try{
    if(save){ const id=save.dataset.saveName, input=document.querySelector(`[data-fan-name="${id}"]`); await api(`/api/fans/${id}/name`,{method:'POST',body:JSON.stringify({name:input.value})}); showToast('Fan name saved'); await refresh(true); }
    if(saveProfile){
      const id=saveProfile.dataset.saveProfile;
      const slider=document.querySelector(`[data-pwm-slider="${id}"]`);
      const startup=document.querySelector(`[data-startup-profile="${id}"]`);
      const savedPercent=Number(slider.value);
      await api(`/api/fans/${id}/profile`,{method:'POST',body:JSON.stringify({percent:savedPercent,restore_on_startup:startup.checked,apply_now:true})});
      pendingPWM.delete(id);
      showToast(`Custom profile saved at ${savedPercent}%`);
      await refresh(true);
    }
    if(clearProfile){
      const id=clearProfile.dataset.clearProfile;
      await api(`/api/fans/${id}/profile`,{method:'DELETE',body:'{}'});
      pendingPWM.delete(id);
      showToast('Saved custom profile cleared');
      await refresh(true);
    }
    if(profile){ await api('/api/profile',{method:'POST',body:JSON.stringify({profile:profile.dataset.profile})}); showToast(`${profile.dataset.profile} profile applied`); await refresh(true); }
    if(restore){ await api(`/api/fans/${restore.dataset.restore}/restore`,{method:'POST',body:'{}'}); showToast('Startup PWM mode restored'); await refresh(true); }
  }catch(err){ showToast(err.message,true); }
});
$('#restoreAll').addEventListener('click',async()=>{ try{ await api('/api/restore',{method:'POST',body:'{}'}); showToast('Startup PWM modes restored'); await refresh(true); }catch(e){showToast(e.message,true);} });
$('#refreshBtn').addEventListener('click',()=>refresh());
refresh(); refreshTimer=setInterval(()=>refresh(true),2000);


document.addEventListener('change', async e=>{
  if(!e.target.matches('[data-startup-profile]')) return;
  const id=e.target.dataset.startupProfile;
  const slider=document.querySelector(`[data-pwm-slider="${id}"]`);
  try {
    await api(`/api/fans/${id}/profile`,{
      method:'POST',
      body:JSON.stringify({
        percent:Number(slider.value),
        restore_on_startup:e.target.checked,
        apply_now:false
      })
    });
    showToast(e.target.checked?'Startup restore enabled':'Startup restore disabled');
    await refresh(true);
  } catch(err) {
    e.target.checked=!e.target.checked;
    showToast(err.message,true);
  }
});

let tempsExpanded=false;
$('#toggleTemps').addEventListener('click',()=>{
  tempsExpanded=!tempsExpanded;
  tempsEl.classList.toggle('collapsed',!tempsExpanded);
  $('#toggleTemps').textContent=tempsExpanded?'Show less':'Show all';
});
