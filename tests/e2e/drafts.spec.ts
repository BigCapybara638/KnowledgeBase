import {test,expect,type Page} from '@playwright/test'
import {login,createArticle} from './helpers'

async function saveDraft(page:Page){
 await page.getByRole('button',{name:'Сохранить черновик',exact:true}).click()
 await expect(page.locator('#editor-status')).toHaveText('Черновик сохранён')
}

async function createDraft(page:Page,title:string,body:string){
 await page.goto('/')
 await page.getByRole('button',{name:'Новая статья'}).click()
 await page.getByLabel('Название статьи').fill(title)
 await page.locator('.tiptap').fill(body)
 await saveDraft(page)
 return new URL(page.url()).pathname
}

function draftCard(page:Page,url:string){
 return page.locator('article.article-card.draft-card').filter({has:page.locator(`a.draft-link[href="${url}"]`)})
}

async function csrfToken(page:Page){
 const token=await page.locator('meta[name="csrf-token"]').getAttribute('content')
 expect(token).toBeTruthy()
 return token!
}

test('cancel preserves an own draft and confirmation removes it',async({page})=>{
 await login(page)
 const draftURL=await createDraft(page,'Черновик для удаления','Текст собственного черновика')
 const unnamedURL=await createDraft(page,'','Черновик без названия остаётся')
 await page.getByRole('link',{name:'Мои черновики',exact:true}).click()
 const card=draftCard(page,draftURL)
 await expect(card.locator('a.draft-link')).toBeVisible()
 await expect(draftCard(page,unnamedURL).getByRole('button',{name:'Удалить черновик «Без названия»',exact:true})).toBeVisible()
 const remove=card.getByRole('button',{name:'Удалить черновик «Черновик для удаления»',exact:true})
 await expect(remove).toHaveText('Удалить')
 let confirmation=''
 page.once('dialog',async dialog=>{confirmation=dialog.type();await dialog.dismiss()})
 await remove.click()
 expect(confirmation).toBe('confirm')
 await page.reload()
 await expect(card).toBeVisible()
 expect((await page.request.get(draftURL)).status()).toBe(200)

 const deleted=page.waitForResponse(response=>new URL(response.url()).pathname===draftURL.replace(/\/edit$/,'/delete')&&response.request().method()==='POST')
 page.once('dialog',dialog=>dialog.accept())
 await remove.click()
 expect((await deleted).status()).toBe(303)
 await expect(page).toHaveURL(/\/drafts$/)
 await expect(card).toHaveCount(0)
 await expect(draftCard(page,unnamedURL)).toBeVisible()
 expect((await page.request.get(draftURL)).status()).toBe(404)
})

test('deleting a private revision keeps the published article',async({page})=>{
 await login(page)
 const articleURL=await createArticle(page,'Опубликованная инструкция остаётся','Исходный опубликованный текст')
 await page.getByRole('button',{name:'Редактировать',exact:true}).click()
 const draftURL=new URL(page.url()).pathname
 await page.getByLabel('Название статьи').fill('Неопубликованная редакция для удаления')
 await page.locator('.tiptap').fill('Эти правки не должны попасть в публикацию')
 await saveDraft(page)
 await page.getByRole('link',{name:'Мои черновики',exact:true}).click()
 page.once('dialog',dialog=>dialog.accept())
 await draftCard(page,draftURL).getByRole('button',{name:'Удалить черновик «Неопубликованная редакция для удаления»',exact:true}).click()
 await expect(page).toHaveURL(/\/drafts$/)
 await expect(draftCard(page,draftURL)).toHaveCount(0)
 expect((await page.request.get(draftURL)).status()).toBe(404)
 await page.goto(articleURL)
 await expect(page.getByRole('heading',{name:'Опубликованная инструкция остаётся',exact:true})).toBeVisible()
 await expect(page.locator('.article-body')).toHaveText('Исходный опубликованный текст')
})

test('an administrator cannot delete another owner’s private draft',async({page,browser,baseURL})=>{
 const ownerContext=await browser.newContext({baseURL})
 try{
  const owner=await ownerContext.newPage()
  await login(owner,'reader')
  const draftURL=await createDraft(owner,'Личный черновик читателя','Приватный текст читателя')
  await owner.reload()
  const state=JSON.parse((await owner.locator('#editor-state').textContent())!)
  await login(page)
  await page.goto('/drafts')
  await expect(page.locator(`a[href="${draftURL}"]`)).toHaveCount(0)
  const response=await page.request.post(`/drafts/${state.id}/delete`,{form:{csrf_token:await csrfToken(page),version:String(state.version)},maxRedirects:0})
  expect(response.status()).toBe(404)
  expect((await page.request.get(draftURL)).status()).toBe(404)
  await owner.reload()
  await expect(owner.locator('.tiptap')).toHaveText('Приватный текст читателя')
 }finally{
  await ownerContext.close()
 }
})

test('draft deletion rejects stale versions and invalid form credentials',async({page})=>{
 await login(page)
 const draftURL=await createDraft(page,'Черновик с параллельными правками','Первоначальный текст')
 const deleteURL=draftURL.replace(/\/edit$/,'/delete')
 await page.goto('/drafts')
 const other=await page.context().newPage()
 try{
  await other.goto(draftURL)
  await other.locator('.tiptap').fill('Новый текст сохранён в другой вкладке')
  await saveDraft(other)
  const rejected=page.waitForResponse(response=>new URL(response.url()).pathname===deleteURL&&response.request().method()==='POST')
  page.once('dialog',dialog=>dialog.accept())
  await draftCard(page,draftURL).getByRole('button',{name:'Удалить черновик «Черновик с параллельными правками»',exact:true}).click()
  expect((await rejected).status()).toBe(409)
  await other.reload()
  await expect(other.locator('.tiptap')).toHaveText('Новый текст сохранён в другой вкладке')

  await page.goto('/drafts')
  const version=await draftCard(page,draftURL).locator('input[name="version"]').inputValue()
  const csrf=await csrfToken(page)
  const missingCSRF=await page.request.post(deleteURL,{form:{version},maxRedirects:0})
  expect(missingCSRF.status()).toBe(403)
  const invalidCSRF=await page.request.post(deleteURL,{form:{version,csrf_token:'invalid-token'},maxRedirects:0})
  expect(invalidCSRF.status()).toBe(403)
  const missingVersion=await page.request.post(deleteURL,{form:{csrf_token:csrf},maxRedirects:0})
  expect(missingVersion.status()).toBe(400)
  const invalidVersion=await page.request.post(deleteURL,{form:{version:'not-a-version',csrf_token:csrf},maxRedirects:0})
  expect(invalidVersion.status()).toBe(400)
  await other.reload()
  await expect(other.locator('.tiptap')).toHaveText('Новый текст сохранён в другой вкладке')

  const deleted=await page.request.post(deleteURL,{form:{version,csrf_token:csrf},maxRedirects:0})
  expect(deleted.status()).toBe(303)
  expect(deleted.headers().location).toBe('/drafts')
  expect((await page.request.get(draftURL)).status()).toBe(404)
 }finally{
  await other.close()
 }
})
