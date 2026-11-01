import {Editor} from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
export function mountEditor(){
 const state=JSON.parse(document.getElementById('editor-state').textContent)
 const title=document.getElementById('article-title'),category=document.getElementById('article-category'),status=document.getElementById('editor-status'),errorBox=document.getElementById('editor-error'),conflict=document.getElementById('conflict-panel')
 const saveButton=document.getElementById('save-draft'),publishButton=document.getElementById('publish-draft')
 let dirty=false,busy=false
 const changed=()=>{dirty=true;status.textContent='Есть несохранённые изменения'}
 const editor=new Editor({element:document.getElementById('article-editor'),extensions:[StarterKit.configure({heading:{levels:[1,2,3]},link:{openOnClick:false}})],content:state.document,editorProps:{attributes:{'aria-label':'Текст статьи','role':'textbox','spellcheck':'true'}},onUpdate:changed})
 title.addEventListener('input',changed);category.addEventListener('change',changed)
 const showError=message=>{errorBox.textContent=message;errorBox.hidden=false}
 async function request(url,method,body){const response=await fetch(url,{method,credentials:'same-origin',headers:{'Content-Type':'application/json','Accept':'application/json','X-CSRF-Token':document.querySelector('meta[name="csrf-token"]').content},body:JSON.stringify(body)});const data=await response.json().catch(()=>({error:'Не удалось прочитать ответ сервера'}));if(!response.ok){if(response.status===409)conflict.hidden=false;throw new Error(data.error||'Не удалось сохранить изменения')}return data}
 async function save(){const data=await request(`/drafts/${state.id}`,'PUT',{title:title.value,categoryId:category.value,expectedVersion:state.version,document:editor.getJSON()});state.version=data.version;dirty=false;status.textContent='Черновик сохранён';return data}
 async function perform(publish){if(busy)return;busy=true;saveButton.disabled=publishButton.disabled=true;errorBox.hidden=true;try{if(publish&&(!title.value.trim()||!category.value)){throw new Error('Для публикации укажите название и категорию статьи.')}await save();if(publish){const data=await request(`/drafts/${state.id}/publish`,'POST',{expectedDraftVersion:state.version,expectedArticleVersion:state.baseVersion});dirty=false;location.assign(data.url)}}catch(e){showError(e.message);status.textContent='Изменения не опубликованы'}finally{busy=false;saveButton.disabled=publishButton.disabled=false}}
 saveButton.addEventListener('click',()=>perform(false));publishButton.addEventListener('click',()=>perform(true))
 document.getElementById('text-style').addEventListener('change',e=>{if(e.target.value==='paragraph')editor.chain().focus().setParagraph().run();else editor.chain().focus().toggleHeading({level:Number(e.target.value.slice(1))}).run()})
 const actions={bold:()=>editor.chain().focus().toggleBold().run(),italic:()=>editor.chain().focus().toggleItalic().run(),underline:()=>editor.chain().focus().toggleUnderline().run(),bulletList:()=>editor.chain().focus().toggleBulletList().run(),orderedList:()=>editor.chain().focus().toggleOrderedList().run(),blockquote:()=>editor.chain().focus().toggleBlockquote().run(),undo:()=>editor.chain().focus().undo().run(),redo:()=>editor.chain().focus().redo().run(),link:()=>{const href=prompt('Адрес ссылки',editor.getAttributes('link').href||'https://');if(href===null)return;if(href==='')editor.chain().focus().unsetLink().run();else editor.chain().focus().setLink({href}).run()}}
 document.querySelectorAll('[data-editor-action]').forEach(button=>button.addEventListener('click',()=>actions[button.dataset.editorAction]?.()))
 editor.on('selectionUpdate',()=>{document.querySelectorAll('[data-editor-action]').forEach(button=>button.setAttribute('aria-pressed',String(editor.isActive(button.dataset.editorAction))))})
 window.addEventListener('beforeunload',event=>{if(dirty){event.preventDefault();event.returnValue=''}})
}
