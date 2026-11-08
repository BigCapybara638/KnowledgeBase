import '@fontsource/inter/latin.css'
import '@fontsource/inter/cyrillic.css'
import './styles.css'
import htmx from 'htmx.org'
htmx.config.allowEval = false
htmx.config.allowScriptTags = false
window.htmx = htmx
document.body.addEventListener('htmx:configRequest', (event) => { event.detail.headers['X-CSRF-Token'] = document.querySelector('meta[name="csrf-token"]')?.content ?? '' })
document.querySelectorAll('[data-copy-link]').forEach(button => button.addEventListener('click',async()=>{try{await navigator.clipboard.writeText(window.location.href);button.textContent='Ссылка скопирована'}catch{document.getElementById('toast').textContent='Скопируйте адрес статьи из строки браузера.'}}))
document.querySelectorAll('form[data-confirm]').forEach(form=>form.addEventListener('submit',event=>{if(!window.confirm(form.dataset.confirm))event.preventDefault()}))
if(document.getElementById('editor-state'))import('./editor.js').then(module=>module.mountEditor())
const topbar=document.querySelector('.topbar')
if(topbar)new ResizeObserver(()=>document.documentElement.style.setProperty('--topbar-height',`${topbar.getBoundingClientRect().height}px`)).observe(topbar)
