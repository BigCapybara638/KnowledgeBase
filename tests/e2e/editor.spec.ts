import {test,expect} from '@playwright/test'
import {login} from './helpers'
import {resolve} from 'node:path'
test('visual editor retains tables and protected uploaded images after publication and reopening',async({page,browser})=>{
 await login(page);await page.getByRole('button',{name:'Новая статья'}).click();await page.getByLabel('Название статьи').fill('Статья с таблицей и изображением');await page.getByRole('combobox',{name:'Категория',exact:true}).selectOption({label:'Инструкции'})
 await expect(page.getByRole('button',{name:'Вставить таблицу',exact:true})).toBeVisible();await page.getByRole('button',{name:'Вставить таблицу',exact:true}).click();await page.locator('.tiptap th').first().locator('p').fill('Код отдела');await page.getByRole('button',{name:'Добавить строку',exact:true}).click();await expect(page.locator('.tiptap tr')).toHaveCount(4)
 await page.locator('#image-upload').setInputFiles(resolve('tests/e2e/fixtures/pixel.png'));await expect(page.locator('.tiptap img')).toBeVisible();const imageURL=await page.locator('.tiptap img').getAttribute('src');expect(imageURL).toMatch(/^\/media\//)
 const guest=await browser.newContext({baseURL:'http://127.0.0.1:4173'});const response=await guest.request.get(imageURL!,{maxRedirects:0});expect(response.status()).not.toBe(200);await guest.close()
 await page.getByRole('button',{name:'Опубликовать',exact:true}).click();await expect(page).toHaveURL(/\/articles\//);await expect(page.locator('.article-body table')).toContainText('Код отдела');await expect(page.locator('.article-body img')).toBeVisible();await page.getByRole('button',{name:'Редактировать',exact:true}).click();await expect(page.locator('.tiptap tr')).toHaveCount(4);await expect(page.locator('.tiptap img')).toBeVisible()
})
