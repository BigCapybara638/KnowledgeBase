import {expect,test} from '@playwright/test'
import {createArticle,login} from './helpers'

test('личный ключ подключает агента к опубликованной статье и отзывается',async({page,request})=>{
 await login(page)
 const title=`Статья для MCP ${Date.now()}`
 const keyName=`Тестовый агент ${Date.now()}`
 const articlePath=await createArticle(page,title,'Проверка доступа агента к базе знаний')
 await page.getByRole('link',{name:'Доступ агента',exact:true}).click()
 await expect(page.getByRole('heading',{name:'Доступ агента',exact:true})).toBeVisible()
 await page.getByLabel('Название ключа',{exact:true}).fill(keyName)
 await page.getByRole('button',{name:'Создать ключ',exact:true}).click()
 const key=await page.getByLabel('Новый ключ MCP',{exact:true}).inputValue()
 expect(key).toMatch(/^kb_mcp_[A-Za-z0-9_-]{43}$/)
 const rpc=async(method:string,params:unknown)=>request.post('/mcp',{
  headers:{Authorization:`Bearer ${key}`,Accept:'application/json, text/event-stream','Mcp-Protocol-Version':'2025-11-25'},
  data:{jsonrpc:'2.0',id:1,method,params},
 })
 const initialize=await rpc('initialize',{protocolVersion:'2025-11-25',capabilities:{},clientInfo:{name:'browser-check',version:'1.0'}})
 expect(initialize.status()).toBe(200)
 const fetched=await rpc('tools/call',{name:'get_article',arguments:{id:articlePath.split('/').pop()}})
 expect(fetched.status()).toBe(200)
 const data=await fetched.json()
 expect(data.result.structuredContent.title).toBe(title)
 expect(data.result.structuredContent.text).toContain('Проверка доступа агента')
 expect(new URL(data.result.structuredContent.url).pathname).toBe(articlePath)
 await page.goto('/settings/mcp')
 await expect(page.getByLabel('Новый ключ MCP',{exact:true})).toHaveCount(0)
 await page.getByRole('button',{name:`Отозвать ключ ${keyName}`,exact:true}).click()
 await expect(page.getByRole('row').filter({hasText:keyName})).toContainText('Отозван')
 const denied=await rpc('tools/list',{})
 expect(denied.status()).toBe(401)
})

test('страница доступа агента доступна с узкого экрана',async({page})=>{
 await page.setViewportSize({width:390,height:844})
 await login(page)
 await expect(page.getByRole('link',{name:'Доступ агента',exact:true})).toBeVisible()
 await page.getByRole('link',{name:'Доступ агента',exact:true}).click()
 await expect(page.getByRole('heading',{name:'Доступ агента',exact:true})).toBeVisible()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth)).toBe(false)
})

test('разрешение ключа позволяет агенту создать личный черновик и опубликовать его через редактор',async({page,request})=>{
 await login(page)
 await page.goto('/settings/mcp')
 const keyName=`Автор MCP ${Date.now()}`
 await page.getByLabel('Название ключа',{exact:true}).fill(keyName)
 await expect(page.getByRole('combobox',{name:'Права ключа',exact:true})).toHaveValue('false')
 await page.getByRole('button',{name:'Создать ключ',exact:true}).click()
 const key=await page.getByLabel('Новый ключ MCP',{exact:true}).inputValue()
 const rpc=async(method:string,params:unknown)=>request.post('/mcp',{
  headers:{Authorization:`Bearer ${key}`,Accept:'application/json, text/event-stream','Mcp-Protocol-Version':'2025-11-25'},
  data:{jsonrpc:'2.0',id:1,method,params},
 })
 expect((await rpc('initialize',{protocolVersion:'2025-11-25',capabilities:{},clientInfo:{name:'write-check',version:'1.0'}})).status()).toBe(200)
 expect((await (await rpc('tools/list',{})).json()).result.tools).toHaveLength(4)
 await page.goto('/settings/mcp')
 await page.getByRole('button',{name:`Разрешить создание черновиков для ключа ${keyName}`,exact:true}).click()
 expect((await (await rpc('tools/list',{})).json()).result.tools).toHaveLength(5)
 const categories=(await (await rpc('tools/call',{name:'list_categories',arguments:{}})).json()).result.structuredContent.items
 const title=`Черновик от агента ${Date.now()}`
 const body='Инструкция от агента 🚀\n\nПроверьте настройки перед публикацией.'
 const created=(await (await rpc('tools/call',{name:'create_article',arguments:{title,body,category_id:categories[0].id}})).json()).result.structuredContent
 expect(created.status).toBe('draft')
 const before=(await (await rpc('tools/call',{name:'search_articles',arguments:{query:title}})).json()).result.structuredContent.items
 expect(before).toHaveLength(0)
 await page.goto(created.edit_url)
 await expect(page.getByLabel('Название статьи',{exact:true})).toHaveValue(title)
 await expect(page.locator('.tiptap')).toContainText('Инструкция от агента 🚀')
 await expect(page.locator('.tiptap')).toContainText('Проверьте настройки перед публикацией.')
 await page.getByRole('button',{name:'Опубликовать',exact:true}).click()
 await expect(page).toHaveURL(new RegExp(`/articles/${created.article_id}$`))
 const published=(await (await rpc('tools/call',{name:'get_article',arguments:{id:created.article_id}})).json()).result.structuredContent
 expect(published.text).toBe(body)
 await page.goto('/settings/mcp')
 await page.getByRole('button',{name:`Запретить создание черновиков для ключа ${keyName}`,exact:true}).click()
 expect((await (await rpc('tools/list',{})).json()).result.tools).toHaveLength(4)
 const denied=await (await rpc('tools/call',{name:'create_article',arguments:{title:'Denied',body:'Denied'}})).json()
 expect(!!denied.error||denied.result?.isError===true).toBe(true)
 await page.getByRole('button',{name:`Отозвать ключ ${keyName}`,exact:true}).click()
})

test('документ MCP сохраняет оформление в редакторе и после публикации',async({page,request})=>{
 await login(page)
 await page.goto('/settings/mcp')
 const keyName=`Форматирование MCP ${Date.now()}`
 await page.getByLabel('Название ключа',{exact:true}).fill(keyName)
 await page.getByRole('combobox',{name:'Права ключа',exact:true}).selectOption('true')
 await page.getByRole('button',{name:'Создать ключ',exact:true}).click()
 const key=await page.getByLabel('Новый ключ MCP',{exact:true}).inputValue()
 const rpc=async(name:string,args:unknown)=>{
  const response=await request.post('/mcp',{
   headers:{Authorization:`Bearer ${key}`,Accept:'application/json, text/event-stream','Mcp-Protocol-Version':'2025-11-25'},
   data:{jsonrpc:'2.0',id:1,method:'tools/call',params:{name,arguments:args}},
  })
  expect(response.status()).toBe(200)
  const data=await response.json()
  expect(data.error).toBeUndefined()
  expect(data.result.isError).not.toBe(true)
  return data.result.structuredContent
 }
 const text=(value:string,marks?:unknown[])=>({type:'text',text:value,...(marks?{marks}:{})})
 const paragraph=(value:string)=>({type:'paragraph',content:[text(value)]})
 const cell=(type:string,value:string,attrs={})=>({type,attrs,content:[paragraph(value)]})
 const document={type:'doc',content:[
  {type:'heading',attrs:{level:2},content:[text('Настройка агента')]},
  {type:'paragraph',content:[text('Цветной текст',[{type:'bold'},{type:'italic'},{type:'underline'},{type:'textStyle',attrs:{color:'#123456',backgroundColor:'#ffeecc'}}])]},
  {type:'paragraph',content:[text('Справка',[{type:'link',attrs:{href:'https://example.com/help'}}])]},
  {type:'paragraph',content:[text('inline code',[{type:'code'}])]},
  {type:'bulletList',content:[{type:'listItem',content:[paragraph('Первый шаг')]}]},
  {type:'orderedList',attrs:{start:2},content:[{type:'listItem',content:[paragraph('Второй шаг')]}]},
  {type:'blockquote',content:[paragraph('Заметка')]},
  {type:'codeBlock',attrs:{language:'sh'},content:[text('echo hello')]},
  {type:'table',content:[
   {type:'tableRow',content:[cell('tableHeader','Параметр'),cell('tableHeader','Значение')]},
   {type:'tableRow',content:[cell('tableCell','Порт'),cell('tableCell','8081',{align:'center',backgroundColor:'#ffeecc',colwidth:[160],height:48})]},
  ]},
 ]}
 const categories=await rpc('list_categories',{})
 const created=await rpc('create_article',{title:`Статья с оформлением ${Date.now()}`,document,category_id:categories.items[0].id})
 await page.goto(created.edit_url)
 const check=async(root:string)=>{
  await expect(page.locator(`${root} h2`)).toHaveText('Настройка агента')
  await expect(page.locator(`${root} strong`)).toContainText('Цветной текст')
  await expect(page.locator(`${root} em`)).toContainText('Цветной текст')
  await expect(page.locator(`${root} u`)).toContainText('Цветной текст')
  const colored=page.locator(`${root} span`).filter({hasText:'Цветной текст'}).last()
  await expect(colored).toHaveCSS('color','rgb(18, 52, 86)')
  await expect(colored).toHaveCSS('background-color','rgb(255, 238, 204)')
  await expect(page.locator(`${root} a`)).toHaveAttribute('href','https://example.com/help')
  await expect(page.locator(`${root} ul`)).toContainText('Первый шаг')
  await expect(page.locator(`${root} ol`)).toHaveAttribute('start','2')
  await expect(page.locator(`${root} blockquote`)).toContainText('Заметка')
  await expect(page.locator(`${root} pre`)).toContainText('echo hello')
  await expect(page.locator(`${root} p code`)).toHaveText('inline code')
  await expect(page.locator(`${root} table tr`)).toHaveCount(2)
  await expect(page.locator(`${root} table th`)).toHaveCount(2)
  const value=page.locator(`${root} table td`).last()
  await expect(value).toHaveText('8081')
  await expect(value).toHaveCSS('background-color','rgb(255, 238, 204)')
  await expect(value).toHaveCSS('text-align','center')
  await expect(value).toHaveAttribute('style',/height:\s*48px/)
 }
 await check('.tiptap')
 await page.getByRole('button',{name:'Сохранить черновик',exact:true}).click()
 await expect(page.locator('#editor-status')).toHaveText('Черновик сохранён')
 await page.reload()
 await check('.tiptap')
 await page.getByRole('button',{name:'Опубликовать',exact:true}).click()
 await expect(page).toHaveURL(new RegExp(`/articles/${created.article_id}$`))
 await check('.article-body')
 const published=await rpc('get_article',{id:created.article_id})
 expect(published.text).toContain('Настройка агента')
 expect(published.text).toContain('8081')
 await page.goto('/settings/mcp')
 await page.getByRole('button',{name:`Отозвать ключ ${keyName}`,exact:true}).click()
})
