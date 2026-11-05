import {Editor} from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import Image from '@tiptap/extension-image'
import {TableKit} from '@tiptap/extension-table'
import {TableMap} from '@tiptap/pm/tables'
import {TextStyle,CellFormatting,Video,normalizeColor,videoURL} from './rich-extensions'
import {importWord} from './word-import'

export function mountEditor(){
 const byID=id=>document.getElementById(id)
 const state=JSON.parse(byID('editor-state').textContent)
 const title=byID('article-title'),category=byID('article-category'),status=byID('editor-status'),errorBox=byID('editor-error'),conflict=byID('conflict-panel')
 const saveButton=byID('save-draft'),publishButton=byID('publish-draft')
 let dirty=false,busy=false,uploads=0
 const changed=()=>{dirty=true;status.textContent='Есть несохранённые изменения'}
 const showError=message=>{errorBox.textContent=message;errorBox.hidden=false}
 const notice=message=>{byID('editor-notice').textContent=message;byID('editor-notice').hidden=!message}
 const editor=new Editor({
  element:byID('article-editor'),
  extensions:[StarterKit.configure({heading:{levels:[1,2,3]},link:{openOnClick:false}}),Image,TableKit.configure({table:{resizable:true,cellMinWidth:24}}),TextStyle,CellFormatting,Video],
  content:state.document,
  editorProps:{attributes:{'aria-label':'Текст статьи',role:'textbox',spellcheck:'true'},
   handlePaste(view,event){
    if(busy||uploads)return false
    const files=[...(event.clipboardData?.files||[])]
    const word=files.find(file=>/\.docx?$/i.test(file.name))
    if(word){event.preventDefault();loadWord(word);return true}
    const html=event.clipboardData?.getData('text/html')
    if(html){event.preventDefault();pasteHTML(html,files);return true}
    const images=files.filter(file=>file.type.startsWith('image/'))
    if(!images.length)return false
    event.preventDefault();insertImages(images);return true
   },
   handleDrop(view,event){
    if(busy||uploads)return false
    const files=[...(event.dataTransfer?.files||[])]
    const word=files.find(file=>/\.docx?$/i.test(file.name))
    const images=files.filter(file=>file.type.startsWith('image/'))
    if(!word&&!images.length)return false
    event.preventDefault()
    const position=view.posAtCoords({left:event.clientX,top:event.clientY})?.pos
    if(position!==undefined)editor.commands.setTextSelection(position)
    if(word)loadWord(word);else insertImages(images)
    return true
   }
  },onUpdate:changed
 })
 title.addEventListener('input',changed);category.addEventListener('change',changed)
 const csrf=()=>document.querySelector('meta[name="csrf-token"]').content
 function lockEditing(locked){
  editor.setEditable(!locked,false);title.disabled=category.disabled=locked
  document.querySelectorAll('.editor-toolbar button,.editor-toolbar select,.editor-toolbar input').forEach(control=>control.disabled=locked)
  saveButton.disabled=publishButton.disabled=locked
 }
 async function request(url,method,body){
  const response=await fetch(url,{method,credentials:'same-origin',headers:{'Content-Type':'application/json',Accept:'application/json','X-CSRF-Token':csrf()},body:JSON.stringify(body)})
  const data=await response.json().catch(()=>({error:'Не удалось прочитать ответ сервера'}))
  if(!response.ok){
   if(response.status===409){
    conflict.hidden=false;const staleDraft=method==='PUT'
    conflict.querySelector('h2').textContent=staleDraft?'Черновик изменён в другой вкладке':'Статья уже изменена'
    conflict.querySelector('p').textContent=staleDraft?'Ваш текст остаётся здесь. Откройте текущий черновик в новой вкладке и перенесите нужные изменения.':'Ваш текст остаётся в редакторе. Сравните его с актуальной публикацией и перенесите нужные изменения в новый черновик.'
    const link=conflict.querySelector('a');link.href=staleDraft?`/drafts/${state.id}/edit`:`/articles/${state.articleId}`
    link.textContent=staleDraft?'Открыть текущий черновик':'Открыть актуальную статью';conflict.querySelector('form').hidden=staleDraft
   }
   throw new Error(data.error||'Не удалось сохранить изменения')
  }
  return data
 }
 async function save(){
  const data=await request(`/drafts/${state.id}`,'PUT',{title:title.value,categoryId:category.value,expectedVersion:state.version,document:editor.getJSON()})
  state.version=data.version;dirty=false;status.textContent='Черновик сохранён';return data
 }
 async function perform(publish){
  if(busy||uploads)return
  busy=true;lockEditing(true);errorBox.hidden=true
  try{
   if(publish&&(!title.value.trim()||!category.value))throw new Error('Для публикации укажите название и категорию статьи.')
   await save()
   if(publish){const data=await request(`/drafts/${state.id}/publish`,'POST',{expectedDraftVersion:state.version,expectedArticleVersion:state.baseVersion});dirty=false;location.assign(data.url)}
  }catch(error){showError(error.message);status.textContent='Изменения не опубликованы'}
  finally{busy=false;lockEditing(false)}
 }
 saveButton.addEventListener('click',()=>perform(false));publishButton.addEventListener('click',()=>perform(true))
 byID('text-style').addEventListener('change',event=>{
  if(event.target.value==='paragraph')editor.chain().focus().setParagraph().run()
  else editor.chain().focus().toggleHeading({level:Number(event.target.value.slice(1))}).run()
 })
 const actions={
  insertTable:()=>editor.chain().focus().insertTable({rows:3,cols:3,withHeaderRow:true}).run(),
  addRow:()=>editor.chain().focus().addRowAfter().run(),deleteRow:()=>editor.chain().focus().deleteRow().run(),
  addColumn:()=>editor.chain().focus().addColumnAfter().run(),deleteColumn:()=>editor.chain().focus().deleteColumn().run(),
  deleteTable:()=>editor.chain().focus().deleteTable().run(),mergeCells:()=>editor.chain().focus().mergeCells().run(),splitCell:()=>editor.chain().focus().splitCell().run(),toggleHeaderRow:()=>editor.chain().focus().toggleHeaderRow().run(),
  bold:()=>editor.chain().focus().toggleBold().run(),italic:()=>editor.chain().focus().toggleItalic().run(),underline:()=>editor.chain().focus().toggleUnderline().run(),
  bulletList:()=>editor.chain().focus().toggleBulletList().run(),orderedList:()=>editor.chain().focus().toggleOrderedList().run(),blockquote:()=>editor.chain().focus().toggleBlockquote().run(),
  undo:()=>editor.chain().focus().undo().run(),redo:()=>editor.chain().focus().redo().run(),
  clearColor:()=>editor.chain().focus().unsetMark('textStyle').run(),
  clearCellColor:()=>editor.chain().focus().setCellAttribute('backgroundColor',null).run(),
  link:()=>{const href=prompt('Адрес ссылки',editor.getAttributes('link').href||'https://');if(href===null)return;if(href==='')editor.chain().focus().unsetLink().run();else editor.chain().focus().setLink({href}).run()},
  video:()=>{const value=prompt('Ссылка на видеофайл MP4, WebM или Ogg (http/https)');if(value===null)return;const src=videoURL(value.trim());if(!src){showError('Укажите корректную ссылку http:// или https:// на видеофайл.');return}editor.chain().focus().insertContent({type:'video',attrs:{src,title:'Видео'}}).run()}
 }
 document.querySelectorAll('[data-editor-action]').forEach(button=>button.addEventListener('click',()=>actions[button.dataset.editorAction]?.()))
 for(const [id,attribute] of [['text-color','color'],['text-background','backgroundColor']])byID(id).addEventListener('input',event=>editor.chain().focus().setMark('textStyle',{[attribute]:event.target.value}).run())
 byID('cell-background').addEventListener('input',event=>editor.chain().focus().setCellAttribute('backgroundColor',event.target.value).run())
 function tableSelection(){
  const {$from}=editor.state.selection
  let depth=$from.depth
  while(depth>0&&$from.node(depth).type.name!=='table')depth--
  if(!depth)return null
  const table=$from.node(depth),start=$from.start(depth),map=TableMap.get(table)
  let cellDepth=$from.depth
  while(cellDepth>depth&&!['tableCell','tableHeader'].includes($from.node(cellDepth).type.name))cellDepth--
  if(cellDepth===depth)return null
  const position=$from.before(cellDepth)-start
  return {table,start,map,rect:map.findCell(position)}
 }
 function setDimension(kind,value){
  const number=Number(value),selected=tableSelection()
  if(!selected){notice('Сначала выберите ячейку таблицы.');return}
  if(!Number.isInteger(number)||number<(kind==='width'?24:16)||number>2000){showError('Размер должен быть целым числом от '+(kind==='width'?24:16)+' до 2000 px.');return}
  const {table,start,map,rect}=selected,tr=editor.state.tr,seen=new Set()
  const positions=kind==='width'?Array.from({length:map.height},(_,row)=>map.map[row*map.width+rect.left]):Array.from({length:map.width},(_,column)=>map.map[rect.top*map.width+column])
  for(const pos of positions){
   if(seen.has(pos))continue;seen.add(pos)
   const cell=table.nodeAt(pos),attrs={...cell.attrs}
   if(kind==='height')attrs.height=number
   else{const widths=attrs.colwidth?[...attrs.colwidth]:Array(attrs.colspan).fill(0);widths[rect.left-map.findCell(pos).left]=number;attrs.colwidth=widths}
   tr.setNodeMarkup(start+pos,undefined,attrs)
  }
  editor.view.dispatch(tr);editor.commands.focus();notice('')
 }
 for(const [id,kind] of [['column-width','width'],['row-height','height']]){
  const control=byID(id)
  control.addEventListener('change',()=>setDimension(kind,control.value))
  control.addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();setDimension(kind,control.value)}})
 }
 editor.on('selectionUpdate',()=>{
  document.querySelectorAll('[data-editor-action]').forEach(button=>button.setAttribute('aria-pressed',String(editor.isActive(button.dataset.editorAction))))
  const style=editor.getAttributes('textStyle')
  if(style.color)byID('text-color').value=style.color
  if(style.backgroundColor)byID('text-background').value=style.backgroundColor
  const cell=editor.isActive('tableHeader')?editor.getAttributes('tableHeader'):editor.getAttributes('tableCell')
  byID('column-width').value=cell.colwidth?.find(Boolean)||'';byID('row-height').value=cell.height||''
  if(cell.backgroundColor)byID('cell-background').value=cell.backgroundColor
 })
 async function uploadMedia(file){
  if(file.size>10*1024*1024)throw new Error('Максимальный размер медиафайла — 10 МиБ.')
  const form=new FormData();form.append('file',file)
  const response=await fetch(`/drafts/${state.id}/media`,{method:'POST',credentials:'same-origin',headers:{'X-CSRF-Token':csrf(),Accept:'application/json'},body:form})
  const data=await response.json().catch(()=>({error:'Не удалось загрузить медиафайл'}))
  if(!response.ok)throw new Error(data.error||'Не удалось загрузить медиафайл')
  return data.src
 }
 async function importing(action,message){
  if(busy||uploads)return
  uploads++;lockEditing(true);errorBox.hidden=true;notice('');status.textContent=message
  try{await action()}catch(error){showError(error.message)}
  finally{uploads--;lockEditing(false);status.textContent=dirty?'Есть несохранённые изменения':'Черновик загружен'}
 }
 function insertImages(files){return importing(async()=>{
  const images=[];for(const file of files)images.push({type:'image',attrs:{src:await uploadMedia(file),alt:file.name}})
  editor.chain().focus().insertContent(images).run()
 },'Загрузка изображений…')}
 function loadWord(file){return importing(async()=>{
  const result=await importWord(file,uploadMedia)
  editor.chain().focus().insertContent(result.html).run()
  notice(['Документ Word вставлен. Проверьте оформление перед публикацией.',...result.warnings].join(' '))
 },'Импорт документа Word…')}
 function pasteHTML(html,files){return importing(async()=>{
  const doc=new DOMParser().parseFromString(html,'text/html'),warnings=[]
  doc.querySelectorAll('script,style,link,iframe,object,embed').forEach(node=>{node.remove();warnings.push('Встроенный объект не перенесён; видео добавляйте через «Видео».')})
  for(const video of doc.querySelectorAll('video')){
   const src=videoURL(video.getAttribute('src')||video.querySelector('source')?.getAttribute('src')||'')
   if(src)video.setAttribute('src',src)
   else{video.remove();warnings.push('Видео из буфера недоступно. Добавьте видео отдельно файлом или по прямой ссылке.')}
  }
  let imageIndex=0
  for(const image of doc.querySelectorAll('img')){
   const src=image.getAttribute('src')||''
   if(/^\/media\/[0-9a-f-]+$/i.test(src))continue
   let file
   if(src.startsWith('data:image/')&&src.length<14*1024*1024){const match=src.match(/^data:(image\/(?:png|jpeg|webp));base64,([\s\S]+)$/);if(match){const bytes=Uint8Array.from(atob(match[2]),c=>c.charCodeAt(0));file=new File([bytes],`word-image.${match[1].split('/')[1]}`,{type:match[1]})}}
   else file=files.filter(file=>file.type.startsWith('image/'))[imageIndex++]
   if(file)image.src=await uploadMedia(file)
   else{image.remove();warnings.push('Изображение из буфера недоступно. Для переноса изображений импортируйте файл .docx.')}
  }
  // Word may put inline colors on a paragraph or a table cell, rather than a span.
  const walker=doc.createTreeWalker(doc.body,NodeFilter.SHOW_TEXT),texts=[]
  while(walker.nextNode())texts.push(walker.currentNode)
  for(const text of texts){
   let color=null,background=null
   for(let parent=text.parentElement;parent&&parent!==doc.body;parent=parent.parentElement){color||=normalizeColor(parent.style.color||parent.getAttribute('color'));background||=normalizeColor(parent.style.backgroundColor)}
   if(color||background){const span=doc.createElement('span');if(color)span.style.color=color;if(background)span.style.backgroundColor=background;text.replaceWith(span);span.append(text)}
  }
  editor.chain().focus().insertContent(doc.body.innerHTML).run();notice([...new Set(warnings)].join(' '))
 },'Вставка форматированного текста…')}
 byID('add-video').addEventListener('click',()=>byID('video-upload').click())
 byID('video-upload').addEventListener('change',event=>{
  const file=event.target.files?.[0]
  if(file)importing(async()=>{const src=await uploadMedia(file);editor.chain().focus().insertContent({type:'video',attrs:{src,title:file.name}}).run()},'Загрузка видео…')
  event.target.value=''
 })
 byID('add-image').addEventListener('click',()=>byID('image-upload').click())
 byID('image-upload').addEventListener('change',event=>{const files=[...(event.target.files||[])];if(files.length)insertImages(files);event.target.value=''})
 byID('import-word').addEventListener('click',()=>byID('word-upload').click())
 byID('word-upload').addEventListener('change',event=>{const file=event.target.files?.[0];if(file)loadWord(file);event.target.value=''})
 window.addEventListener('beforeunload',event=>{if(dirty||uploads){event.preventDefault();event.returnValue=''}})
}
