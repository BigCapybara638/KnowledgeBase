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
