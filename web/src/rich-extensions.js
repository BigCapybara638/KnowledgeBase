import {Extension, Mark, Node, mergeAttributes} from '@tiptap/core'

export function normalizeColor(value){
 if(!value || /var\(|currentcolor|transparent/i.test(value))return null
 const context=document.createElement('canvas').getContext('2d')
 if(!CSS.supports('color',value))return null
 context.fillStyle=value
 const result=context.fillStyle
 if(/^#[0-9a-f]{6}$/i.test(result))return result.toLowerCase()
 return null
}
export function dimension(value){
 if(!value)return null
 const match=String(value).match(/^([\d.]+)(px|pt)?$/)
 if(!match)return null
 const number=Math.round(Number(match[1])*(match[2]==='pt'?4/3:1))
 return number>=16&&number<=2000?number:null
}
export function videoURL(value){
 if(/^\/media\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value))return value
 try{const url=new URL(value);return ['https:','http:'].includes(url.protocol)&&!url.username&&!url.password&&!/[\r\n\\]/.test(value)?url.href:null}catch{return null}
}
export const TextStyle=Mark.create({
 name:'textStyle',
 addAttributes(){return {
  color:{default:null,parseHTML:element=>normalizeColor(element.style.color),renderHTML:attrs=>attrs.color?{style:`color:${attrs.color}`} :{}},
  backgroundColor:{default:null,parseHTML:element=>normalizeColor(element.style.backgroundColor),renderHTML:attrs=>attrs.backgroundColor?{style:`background-color:${attrs.backgroundColor}`} :{}}
 }},
 parseHTML(){return [{tag:'span',getAttrs:element=>element.style.color||element.style.backgroundColor?null:false},{tag:'font',getAttrs:element=>element.getAttribute('color')?{color:normalizeColor(element.getAttribute('color'))}:false},{tag:'mark',getAttrs:element=>({backgroundColor:normalizeColor(element.style.backgroundColor)||'#ffff00'})}]},
 renderHTML({HTMLAttributes}){return ['span',mergeAttributes(HTMLAttributes),0]}
})
export const CellFormatting=Extension.create({
 name:'cellFormatting',
 addGlobalAttributes(){return [{types:['tableCell','tableHeader'],attributes:{
  backgroundColor:{default:null,parseHTML:element=>normalizeColor(element.style.backgroundColor||element.getAttribute('bgcolor')),renderHTML:attrs=>attrs.backgroundColor?{style:`background-color:${attrs.backgroundColor}`} :{}},
  height:{default:null,parseHTML:element=>dimension(element.style.height||element.getAttribute('height')||element.parentElement?.style.height),renderHTML:attrs=>attrs.height?{style:`height:${attrs.height}px`}:{}},
  colwidth:{default:null,parseHTML:element=>{
   const span=Math.max(1,Math.min(100,Number(element.getAttribute('colspan'))||1))
   const raw=element.getAttribute('data-colwidth')||element.getAttribute('colwidth')
   if(raw){const widths=raw.split(',').map(Number);if(widths.length===span&&widths.every(v=>Number.isInteger(v)&&v>=0&&v<=2000))return widths}
   const width=dimension(element.style.width||element.getAttribute('width'))
   if(!width)return null
   const base=Math.floor(width/span),remainder=width%span
   return Array.from({length:span},(_,index)=>base+(index<remainder?1:0))
  },renderHTML:attrs=>attrs.colwidth?{'data-colwidth':attrs.colwidth.join(','),style:`width:${attrs.colwidth.reduce((a,b)=>a+b,0)}px`}: {}}
 }}]}
})
export const Video=Node.create({
 name:'video',group:'block',atom:true,draggable:true,
 addAttributes(){return {src:{default:null},title:{default:null}}},
 parseHTML(){return [{tag:'video',getAttrs:element=>{const src=videoURL(element.getAttribute('src')||element.querySelector('source')?.getAttribute('src'));return src?{src,title:element.getAttribute('title')}:false}}]},
 renderHTML({HTMLAttributes}){return ['video',mergeAttributes(HTMLAttributes,{controls:'',preload:'metadata'})]}
})
