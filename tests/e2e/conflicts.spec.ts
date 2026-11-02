import {test,expect} from '@playwright/test'
import {login,createArticle} from './helpers'
test('concurrent authors keep their text and only one publication wins',async({page,browser})=>{
 await login(page);const url=await createArticle(page,'Конкурентная публикация','Исходный текст');await page.getByRole('button',{name:'Редактировать',exact:true}).click();await page.locator('.tiptap').fill('Первый редактор опубликовал этот текст')
 const second=await browser.newContext({baseURL:'http://127.0.0.1:4173'});const other=await second.newPage();await login(other,'reader');await other.goto(url);await other.getByRole('button',{name:'Редактировать',exact:true}).click();await other.locator('.tiptap').fill('Второй редактор сохраняет свои изменения')
 await page.getByRole('button',{name:'Опубликовать',exact:true}).click();await expect(page).toHaveURL(/\/articles\//)
 await other.getByRole('button',{name:'Опубликовать',exact:true}).click();await expect(other.locator('#conflict-panel')).toBeVisible();await expect(other.locator('.tiptap')).toContainText('Второй редактор сохраняет свои изменения')
 await page.reload();await expect(page.locator('.article-body')).toContainText('Первый редактор опубликовал этот текст');await second.close()
})

test('two tabs of a private draft retain the second text and link to the current draft',async({page})=>{
 await login(page);await page.getByRole('button',{name:'Новая статья'}).click();await page.getByLabel('Название статьи').fill('Личный черновик');await page.locator('.tiptap').fill('Исходное');await page.getByRole('button',{name:'Сохранить черновик',exact:true}).click();await expect(page.locator('#editor-status')).toHaveText('Черновик сохранён');const draftURL=new URL(page.url()).pathname
 const other=await page.context().newPage();await other.goto(draftURL);await expect(other.locator('.tiptap')).toBeVisible();await page.locator('.tiptap').fill('Первый сохранённый текст');await page.getByRole('button',{name:'Сохранить черновик',exact:true}).click();await expect(page.locator('#editor-status')).toHaveText('Черновик сохранён');await other.locator('.tiptap').fill('Второй текст для переноса');await other.getByRole('button',{name:'Сохранить черновик',exact:true}).click();await expect(other.locator('#conflict-panel')).toBeVisible();await expect(other.locator('#conflict-panel a')).toHaveAttribute('href',draftURL);await expect(other.locator('.tiptap')).toContainText('Второй текст для переноса');await other.close()
})
