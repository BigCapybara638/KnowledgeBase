import {test, expect} from '@playwright/test'
import {login, createArticle} from './helpers'

test('article navigation updates sidebar, works on mobile, and returns home', async ({page}) => {
 await login(page)
 const first = await createArticle(page, '000 Navigation A', 'Первая статья')
 const middle = await createArticle(page, '000 Navigation B', 'Средняя статья')
 const last = await createArticle(page, '000 Navigation C', 'Последняя статья')
 await page.goto(middle)
 const active = page.locator('.sidebar-article[aria-current="page"]')
 await expect(active).toHaveCount(1)
 await expect(active).toHaveAttribute('href', middle)
 const nav = page.getByRole('navigation', {name:'Навигация по статьям'})
 await expect(nav.locator('[rel="prev"]')).toHaveAttribute('href', first)
 await expect(nav.locator('[rel="next"]')).toHaveAttribute('href', last)
 for (const colorScheme of ['light', 'dark'] as const) {
  await page.emulateMedia({colorScheme})
  expect(await active.evaluate(el => getComputedStyle(el).backgroundColor)).not.toBe('rgba(0, 0, 0, 0)')
  for (const width of [1440, 390]) {
   await page.setViewportSize({width, height:900})
   await nav.scrollIntoViewIfNeeded()
   await expect(nav.locator('[rel="prev"]')).toBeVisible()
   await expect(nav.locator('[rel="next"]')).toBeVisible()
   expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
   const article = await page.locator('main article').boundingBox()
   const navigation = await nav.boundingBox()
   expect(navigation!.y).toBeGreaterThanOrEqual(article!.y + article!.height)
   await page.screenshot({path:`.validation/navigation-${colorScheme}-${width}.png`, fullPage:true})
  }
 }
 await nav.locator('[rel="next"]').click()
 await expect(page).toHaveURL(new RegExp(last + '$'))
 await expect(active).toHaveAttribute('href', last)
 await page.locator('[rel="prev"]').click()
 await expect(page).toHaveURL(new RegExp(middle + '$'))
 await expect(active).toHaveAttribute('href', middle)
 await page.locator('.article-actions > a').click()
 await expect(page).toHaveURL('/')
 await expect(page.getByRole('heading', {name:'Статьи', exact:true})).toBeVisible()
 await expect(page.locator('.sidebar-article[aria-current="page"]')).toHaveCount(0)
 await expect(page.locator('a[href*="?category="]')).toHaveCount(0)
 const menuURL = await page.locator('.category-group summary').first().getAttribute('hx-get')
 const category = menuURL!.split('/')[2]
 await page.goto(`/?category=${category}`)
 await expect(page).toHaveURL('/')
 await expect(page.getByRole('link', {name:'Управление категориями'})).toBeVisible()
})
