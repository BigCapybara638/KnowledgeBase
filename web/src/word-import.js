import {Unzip,UnzipInflate,strFromU8} from 'fflate'

const W='http://schemas.openxmlformats.org/wordprocessingml/2006/main'
const R='http://schemas.openxmlformats.org/officeDocument/2006/relationships'
const highlights={yellow:'ffff00',green:'00ff00',cyan:'00ffff',magenta:'ff00ff',blue:'0000ff',red:'ff0000',darkBlue:'000080',darkCyan:'008080',darkGreen:'008000',darkMagenta:'800080',darkRed:'800000',darkYellow:'808000',darkGray:'808080',lightGray:'c0c0c0',black:'000000',white:'ffffff'}
const attr=(element,name)=>element?.getAttributeNS(W,name)||element?.getAttribute(`w:${name}`)||''
const children=(element,name)=>[...(element?.children||[])].filter(e=>e.namespaceURI===W&&e.localName===name)
const child=(element,name)=>children(element,name)[0]
const descendants=(element,name)=>[...(element?.getElementsByTagNameNS(W,name)||[])]
const hex=value=>/^[0-9a-f]{6}$/i.test(value)?`#${value.toLowerCase()}`:null
const px=value=>{const n=Math.round(Number(value)/15);return n>=16&&n<=2000?n:null}
const el=(tag,text)=>{const node=document.createElement(tag);if(text)node.textContent=text;return node}
const safeURL=value=>{try{const u=new URL(value);return ['http:','https:'].includes(u.protocol)&&!u.username&&!u.password?u.href:null}catch{return null}}
const isVideo=value=>/\.(mp4|webm|ogv)(?:[?#]|$)/i.test(value)

function readArchive(buffer){
 const files=new Map();let total=0,count=0
 const unzip=new Unzip(file=>{
  if(++count>512 || (file.originalSize||0)>16*1024*1024)throw new Error('Документ Word слишком большой после распаковки.')
  const chunks=[];let length=0
  file.ondata=(error,data,final)=>{
   if(error)throw error
   total+=data.length;length+=data.length
   if(total>32*1024*1024)throw new Error('Распакованный документ превышает 32 МиБ.')
   chunks.push(data)
   if(final){if(files.has(file.name))throw new Error('Повторяющиеся файлы в документе Word.');const bytes=new Uint8Array(length);let offset=0;for(const chunk of chunks){bytes.set(chunk,offset);offset+=chunk.length}files.set(file.name,bytes)}
  }
  file.start()
 })
 unzip.register(UnzipInflate)
 const bytes=new Uint8Array(buffer)
 for(let offset=0;offset<bytes.length;offset+=16384)unzip.push(bytes.subarray(offset,offset+16384),offset+16384>=bytes.length)
 return files
}
function parseXML(bytes){
 if(!bytes||bytes.length>4*1024*1024)throw new Error('В документе нет корректного XML Word.')
 const xml=strFromU8(bytes)
 if(/<!DOCTYPE|<!ENTITY/i.test(xml))throw new Error('XML с внешними сущностями не поддерживается.')
 const doc=new DOMParser().parseFromString(xml,'application/xml')
 if(doc.querySelector('parsererror'))throw new Error('Повреждённый XML в документе Word.')
 return doc
}

// Returns a detached, restricted DOM tree. No imported HTML is executed.
export async function importWord(file,uploadImage){
 if(!/\.docx$/i.test(file.name))throw new Error('Выберите файл .docx. Старый формат .doc сначала сохраните в Word как .docx.')
 if(file.size>10*1024*1024)throw new Error('Максимальный размер файла Word — 10 МиБ.')
 let files
 try{files=readArchive(await file.arrayBuffer())}catch(error){throw new Error(`Не удалось открыть Word: ${error.message}`)}
 const doc=parseXML(files.get('word/document.xml'))
 const body=doc.getElementsByTagNameNS(W,'body')[0]
 if(!body)throw new Error('В документе нет содержимого Word.')
 const relationships=new Map()
 if(files.has('word/_rels/document.xml.rels'))for(const relation of parseXML(files.get('word/_rels/document.xml.rels')).getElementsByTagNameNS('*','Relationship'))relationships.set(relation.getAttribute('Id'),{target:relation.getAttribute('Target'),external:relation.getAttribute('TargetMode')==='External',type:relation.getAttribute('Type')||''})
 const numbering=new Map()
 if(files.has('word/numbering.xml')){
  const definitions=parseXML(files.get('word/numbering.xml'))
  const abstracts=new Map(descendants(definitions,'abstractNum').map(node=>[attr(node,'abstractNumId'),node]))
  for(const number of descendants(definitions,'num')){
   const abstract=abstracts.get(attr(child(number,'abstractNumId'),'val'))
   const levels=new Map()
   for(const level of children(abstract,'lvl'))levels.set(Number(attr(level,'ilvl')),{tag:attr(child(level,'numFmt'),'val')==='bullet'?'ul':'ol',start:Math.max(1,Number(attr(child(level,'start'),'val'))||1)})
   numbering.set(attr(number,'numId'),levels)
  }
 }
 const warnings=new Set(),images=new Map()
 const root=el('div')
 // Build the complete document before uploading, so malformed XML cannot partly replace content.
 function formatRun(run){
  let result=el('span');const properties=child(run,'rPr')
  const color=hex(attr(child(properties,'color'),'val'))
  const background=hex(attr(child(properties,'shd'),'fill'))||hex(highlights[attr(child(properties,'highlight'),'val')])
  if(color)result.style.color=color
  if(background)result.style.backgroundColor=background
  for(const source of run.childNodes){
   if(source.nodeType!==1)continue
   const name=source.localName
   if(name==='t'||name==='delText')result.append(document.createTextNode(source.textContent))
   else if(name==='tab')result.append(document.createTextNode('\t'))
   else if(name==='br'||name==='cr')result.append(el('br'))
   else if(name==='drawing'||name==='pict'){
    const blip=source.getElementsByTagNameNS('*','blip')[0]||source.getElementsByTagNameNS('*','imagedata')[0]
    const relation=relationships.get(blip?.getAttributeNS(R,'embed')||blip?.getAttributeNS(R,'id'))
    if(relation&&!relation.external){
     const path=new URL(relation.target,'https://word.local/word/document.xml').pathname.slice(1)
     const bytes=files.get(path),extension=path.split('.').pop().toLowerCase()
     const type={png:'image/png',jpg:'image/jpeg',jpeg:'image/jpeg',webp:'image/webp'}[extension]
     if(bytes&&type){const image=el('img');image.alt='Изображение из Word';image.dataset.wordImage=path;images.set(path,{bytes,type});result.append(image)}
     else warnings.add('Некоторые изображения имеют неподдерживаемый формат; используйте PNG, JPEG или WebP.')
    }else warnings.add('Внешнее изображение Word не загружено. Добавьте его отдельно.')
   }else if(name==='object')warnings.add('Встроенные объекты и видео Word нужно добавить отдельно через «Видео».')
  }
  for(const [property,tag] of [['b','strong'],['i','em'],['u','u'],['strike','s']]){
   const setting=child(properties,property),value=attr(setting,'val')
   if(setting&&!['0','false','off','none'].includes(value)){const wrapper=el(tag);wrapper.append(result);result=wrapper}
  }
  return result
 }
 function paragraph(source){
  const properties=child(source,'pPr'),style=attr(child(properties,'pStyle'),'val')
  const heading=style.match(/(?:Heading|Заголовок)\s*([1-3])/i)
  const result=el(heading?`h${heading[1]}`:'p')
  function inline(source,target){for(const item of source.children){
   if(item.localName==='r')target.append(formatRun(item))
   else if(item.localName==='hyperlink'){
    const relation=relationships.get(item.getAttributeNS(R,'id')),href=safeURL(relation?.target)
    if(href&&isVideo(href)){const video=el('video');video.src=href;video.controls=true;video.title=item.textContent;target.append(video)}
    else{const link=href?el('a'):el('span');if(href)link.href=href;inline(item,link);target.append(link)}
   }else if(['sdt','sdtContent','smartTag','ins'].includes(item.localName))inline(item,target)
  }}
  inline(source,result)
  const numbering=child(properties,'numPr')
  if(numbering){result.dataset.wordList=attr(child(numbering,'numId'),'val')||'1';result.dataset.wordLevel=String(Math.min(8,Math.max(0,Number(attr(child(numbering,'ilvl'),'val'))||0)))}
  return result
 }
 function blocks(source,target){
  let stack=[]
  for(const item of source.children){
   if(item.localName==='p'){
    const paragraphNode=paragraph(item)
    const videoNodes=[...paragraphNode.querySelectorAll('video')]
    for(const video of videoNodes)video.remove()
    if(paragraphNode.dataset.wordList){
     const id=paragraphNode.dataset.wordList,level=Number(paragraphNode.dataset.wordLevel)
     const definition=numbering.get(id)?.get(level)||{tag:'ul',start:1}
     stack=stack.slice(0,level+1)
     if(stack[level]?.id!==id||stack[level]?.node.tagName.toLowerCase()!==definition.tag)stack=stack.slice(0,level)
     while(stack.length<=level){
      const depth=stack.length,format=numbering.get(id)?.get(depth)||definition,list=el(format.tag)
      if(format.tag==='ol'&&format.start>1)list.setAttribute('start',format.start)
      const parent=stack[depth-1]
      if(parent){if(!parent.last){parent.last=el('li');parent.last.append(el('p'));parent.node.append(parent.last)}parent.last.append(list)}else target.append(list)
      stack.push({id,node:list,last:null})
     }
     const li=el('li');li.append(paragraphNode);stack[level].node.append(li);stack[level].last=li
    }else{stack=[];target.append(paragraphNode)}
    for(const video of videoNodes)target.append(video)
   }else if(item.localName==='tbl'){stack=[];target.append(table(item))}
   else if(['sdt','sdtContent'].includes(item.localName)){stack=[];blocks(item,target)}
  }
 }
 function table(source){
  const result=el('table'),tbody=el('tbody');result.append(tbody)
  const grid=children(child(source,'tblGrid'),'gridCol').map(c=>px(attr(c,'w')))
  let prior=new Map()
  for(const row of children(source,'tr')){
   const tr=el('tr');tbody.append(tr);let column=0;const current=new Map()
   const height=px(attr(child(child(row,'trPr'),'trHeight'),'val'))
   for(const cell of children(row,'tc')){
    const properties=child(cell,'tcPr'),span=Math.min(100,Math.max(1,Number(attr(child(properties,'gridSpan'),'val'))||1))
    const merge=child(properties,'vMerge')
    if(merge&&attr(merge,'val')!=='restart'&&prior.has(column)){
     const previous=prior.get(column);previous.rowSpan+=1;current.set(column,previous);column+=span;continue
    }
    const td=el(child(child(row,'trPr'),'tblHeader')?'th':'td');td.colSpan=span
    const widthType=attr(child(properties,'tcW'),'type')
    const width=widthType==='dxa'?px(attr(child(properties,'tcW'),'w')):grid[column]
    const widths=grid.slice(column,column+span)
    if(span===1&&width)td.dataset.colwidth=String(width)
    else if(widths.length===span&&widths.every(Boolean))td.dataset.colwidth=widths.join(',')
    if(height)td.style.height=`${height}px`
    const background=hex(attr(child(properties,'shd'),'fill'));if(background)td.style.backgroundColor=background
    blocks(cell,td);if(!td.childNodes.length)td.append(el('p'));tr.append(td)
    if(merge)current.set(column,td)
    column+=span
   }
   prior=current
  }
  return result
 }
 blocks(body,root)
 if(doc.getElementsByTagNameNS('*','altChunk').length||doc.getElementsByTagNameNS('*','OLEObject').length)warnings.add('Встроенные объекты Word не переносятся автоматически. Проверьте содержимое и добавьте видео отдельно.')
 const count=root.querySelectorAll('*').length
 if(count>10000)throw new Error('Слишком большой документ: более 10 000 элементов.')
 for(const [path,image] of images){
  const src=await uploadImage(new File([image.bytes],path.split('/').pop(),{type:image.type}))
  for(const node of root.querySelectorAll('img[data-word-image]'))if(node.dataset.wordImage===path){node.src=src;delete node.dataset.wordImage}
 }
 for(const p of root.querySelectorAll('[data-word-list]')){delete p.dataset.wordList;delete p.dataset.wordLevel}
 if(!root.textContent.trim()&&!root.querySelector('img,video,table'))throw new Error('Документ Word не содержит поддерживаемого содержимого.')
 return {html:root.innerHTML,warnings:[...warnings]}
}
