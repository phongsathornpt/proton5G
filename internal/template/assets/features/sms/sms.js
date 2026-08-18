(function(){
    function el(id){return document.getElementById(id)}
    function headers(json){return typeof apiHeaders==='function'?apiHeaders(json):(json?{'Content-Type':'application/json'}:{})}
    function text(id,value){const n=el(id);if(n)n.textContent=value||''}
    function escapeHTML(v){const d=document.createElement('div');d.textContent=v==null?'':String(v);return d.innerHTML}
    function estimate(body){
        const chars=[...body];
        const unicode=chars.some(ch=>{const c=ch.codePointAt(0);return c<0x20||c>0x7e});
        const utf16Units=body.length;
        if(!unicode&&chars.length<=160)return {encoding:'GSM-7',segments:1,chars:chars.length};
        if(unicode&&utf16Units<=70)return {encoding:'UCS2',segments:1,chars:chars.length};
        return {encoding:'UCS2 multipart',segments:Math.ceil(utf16Units/67),chars:chars.length};
    }
    function updateMeta(){const body=(el('sms-body')||{}).value||'';const m=estimate(body);text('sms-meta',`${m.encoding} · ${m.chars} chars · ${m.segments} segment${m.segments===1?'':'s'}`)}
    async function refreshSMS(){
        text('sms-status','Loading…');
        try{
            const resp=await fetch('/api/sms',{headers:headers(false)});
            const data=await resp.json().catch(()=>({}));
            if(!resp.ok)throw new Error(data.error||data.message||`HTTP ${resp.status}`);
            const list=el('sms-list');if(!list)return;
            const msgs=data.messages||[];
            list.innerHTML=msgs.length?'':'<div class="muted">No stored SMS messages.</div>';
            msgs.forEach(msg=>{
                const item=document.createElement('div');item.className='sms-item';
                item.innerHTML=`<div class="sms-item-head"><strong>${escapeHTML(msg.address||'Unknown')}</strong><span class="muted">#${msg.index} · ${escapeHTML(msg.status||'')}</span></div><div class="muted">${escapeHTML(msg.timestamp||'')}</div><div class="sms-item-body">${escapeHTML(msg.body||'')}</div><div class="sms-actions"><button type="button" class="btn secondary sms-delete" data-index="${msg.index}">Delete</button></div>`;
                list.appendChild(item);
            });
            list.querySelectorAll('.sms-delete').forEach(btn=>btn.addEventListener('click',()=>deleteSMS(btn.dataset.index)));
            text('sms-status',`${msgs.length} message${msgs.length===1?'':'s'}`);
        }catch(e){text('sms-status','SMS error: '+e.message)}
    }
    async function deleteSMS(index){
        if(!confirm(`Delete SMS #${index}?`))return;
        try{
            const resp=await fetch(`/api/sms/${encodeURIComponent(index)}`,{method:'DELETE',headers:headers(false)});
            if(!resp.ok)throw new Error((await resp.text())||`HTTP ${resp.status}`);
            await refreshSMS();
        }catch(e){text('sms-status','Delete failed: '+e.message)}
    }
    async function sendSMS(){
        const to=(el('sms-to')||{}).value||'',body=(el('sms-body')||{}).value||'';
        if(!to.trim()||!body.trim()){text('sms-send-status','Phone number and message are required.');return}
        const btn=el('sms-send-btn');if(btn)btn.disabled=true;text('sms-send-status','Sending…');
        try{
            const wc=window.crypto;
            const idem=(wc&&typeof wc.randomUUID==='function')?wc.randomUUID():`${Date.now()}-${Math.random()}`;
            const resp=await fetch('/api/sms/send',{method:'POST',headers:Object.assign(headers(true),{'Idempotency-Key':idem}),body:JSON.stringify({to,body})});
            const data=await resp.json().catch(()=>({}));
            if(!resp.ok)throw new Error(data.error||data.message||`HTTP ${resp.status}`);
            text('sms-send-status',`Sent · ${data.encoding||''} · ${data.segments||1} segment${data.segments===1?'':'s'}`);
            if(el('sms-body'))el('sms-body').value='';updateMeta();
        }catch(e){text('sms-send-status','Send failed: '+e.message)}finally{if(btn)btn.disabled=false}
    }
    document.addEventListener('DOMContentLoaded',()=>{
        const refresh=el('sms-refresh-btn'),send=el('sms-send-btn'),body=el('sms-body');
        if(refresh)refresh.addEventListener('click',refreshSMS);if(send)send.addEventListener('click',sendSMS);if(body)body.addEventListener('input',updateMeta);updateMeta();
    });
    window.refreshSMS=refreshSMS;
})();
