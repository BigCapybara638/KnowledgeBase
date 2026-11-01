import {test,expect} from '@playwright/test'
import {login,createArticle} from './helpers'
test('concurrent authors keep their text and only one publication wins',async({page,browser})=>{
 await login(page);const url=await createArticle(page,'Конкурентная публикация','Исходный текст');await page.getByRole('button',{name:'Редактировать',exact:true}).click();await page.locator('.tiptap').fill('Первый редактор опубликовал этот текст')
 const second=await browser.newContext({baseURL:'http://127.0.0.1:4173'});const other=await second.newPage();await login(other,'reader');await other.goto(url);await other.getByRole('button',{name:'Редактировать',exact:true}).click();await other.locator('.tiptap').fill('Второй редактор сохраняет свои изменения')
 await page.getByRole('button',{name:'Опубликовать',exact:true}).click();await expect(page).toHaveURL(/\/articles\//)
 await other.getByRole('button',{name:'Опубликовать',exact:true}).click();await expect(other.locator('#conflict-panel')).toBeVisible();await expect(other.locator('.tiptap')).toContainText('Второй редактор сохраняет свои изменения')
 await page.reload();await expect(page.locator('.article-body')).toContainText('Первый редактор опубликовал этот текст');await second.close()
})
