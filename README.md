# Instructions
- Clone the repo and run `yarn install`
- Navigate to your course and open dev console.
- Put the `remember_user_token` cookie from your browser in the `cookie` constant of index.js.
- Open network tab and look closely for the first few XHR GET requests. One of them should contain all the metadata and the name of the request is the same as the course's slug.
- Copy the whole response and paste it in a new file in the codebase called `response.json`.
- Create a folder named jsons.
- Run the index.js and you should get a json file in jsons folder containing all wistia code and other metadatas from all the videos in that course.
- Head towards [Wistia-go](https://github.com/KnockOutEZ/wisty-go) repo, clone, install it, get back to the ardan-labs-course-scraper and run `wisty-go --jsons=true`. It will look for ./jsons folder, iterate over every json in it and download all the courses, videos.
- Enjoy!!
