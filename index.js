const puppeteer = require('puppeteer-extra');
const ppUserPrefs = require('puppeteer-extra-plugin-user-preferences');
const fs = require('fs').promises;

// Replace this URL with the actual URL
const urlTemplate = "https://courses.ardanlabs.com/courses/take/%s/lessons/%s";

(async () => {
  puppeteer.use(ppUserPrefs({
    userPrefs: {
      devtools: {
        preferences: {
          currentDockState: "undocked"
        },
      },
    }
  }));
  // Launch a headless browser
  const browser = await puppeteer.launch({
    headless: true,
    devtools: true
  });

  // Create a new page
  const page = await browser.newPage();

  // Set up a custom network condition with the given cookie
  const cookie = ""; // Replace this with the actual cookie
  await page.setCookie({ name: "remember_user_token", value: cookie, domain: "courses.ardanlabs.com", secure: true, path: "/" });

  // Read JSON file
  const jsonData = require('./response.json');

  // Extract relevant data
  const courseSlug = jsonData.course.slug;
  const contents = jsonData.contents;

  const dynamicPartsData = [];

  // Iterate through contents array
  for (const [index, content] of contents.entries()) {
    const contentSlug = content.slug;
    const contentUrl = urlTemplate.replace('%s', courseSlug).replace('%s', contentSlug);

    try {
      // Open the URL in the browser
      await page.goto(contentUrl, { waitUntil: 'domcontentloaded' });

      // Add a delay to allow page content to load (you can adjust this duration)
      await page.waitForTimeout(5000);

      // Capture a screenshot or do any other actions as needed
      // ...

      // log all the script tags on the page
      await page.waitForSelector('script');
      // const scripts = await page.$$eval('script', scripts => scripts.map(s => s.src));
      // console.log(scripts);

      const iframeHandle = await page.$('iframe');
      if (!iframeHandle) {
        console.error(`No iframe found in ${contentUrl}`);
        continue;
      }

      // Get the content frame
      const frame = await iframeHandle.contentFrame();

      // Evaluate a function in the iframe context to log the script tags
      const dynamicParts = await frame.evaluate(() => {
        const scriptElements = document.querySelectorAll('script');
        const dynamicParts = Array.from(scriptElements)
          .filter(s => s.src.startsWith('https://fast.wistia.com/embed/medias/'))
          .map(s => {
            const url = new URL(s.src);
            let dynamicPart = url.pathname.split('/')[3]; // Assuming the dynamic part is at the 4th segment
            dynamicPart = dynamicPart.replace('.jsonp', ''); // Remove the .jsonp part
            return dynamicPart;
          });
        return dynamicParts;
      });

      // Log the dynamic parts in the Node.js context
      console.log(dynamicParts);

      if (dynamicParts.length > 0) {
        dynamicPartsData.push({
          "index": index,
          "dynamic-part": dynamicParts[0],
          "downloaded": false,
          "name": `${index}_${content.name}`
        });
      }
      await page.click('[data-qa="complete-continue__btn"]');




      // const scripts = await page.evaluate(() => {
      //   const scriptElements = document.querySelectorAll('script');
      //   return Array.from(scriptElements).map(s => s.src);
      // });
      // console.log(scripts);
    } catch (error) {
      console.error(`Error processing ${contentUrl}: ${error.message}`);
    }
  }

  // Save dynamic parts data to a JSON file
  const jsonFileName = "./jsons/" + jsonData.course.name + '.json';
  await fs.writeFile(jsonFileName, JSON.stringify({"name":jsonData.course.name, "item-count":dynamicPartsData.length, "items":dynamicPartsData}, null, 2), 'utf-8');
  console.log(`Dynamic parts data saved to ${jsonFileName}`);

  // Close the browser
  await browser.close();
})();
