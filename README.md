# mahresources

Just a simple CRUD app written in golang to serve my personal information management needs. 
It's surely an overkill for any imagination of my requirements but the thing is I like developing software more than
using software.

This thing has support for notes, resources (files), tags, groups (generic entity). Everything is taggable. 
Notes, resources and groups can have json metadata that's editable and queryable via the web GUI.
See the models folder for exact relation definitions.

Supports postgres and sqlite only. Mysql support should be fairly easy to add, I just personally don't care about it.

it has some minimal javascript. 90% of the things work without js too. usually.

## build

First, set the necessary env variables that are catalogued in the `.env.template` file. You can also create a 
copy of the file, rename it to `.env` and customize the values.

I use compiledaemon for continuous builds as I develop:

`CompileDaemon -exclude-dir=".git" -exclude-dir="node_modules" -command="./mahresources" -build="go build --tags json1"`

Just do `go build --tags json1` if you want to have a single build.

To build css, install node, run `npm ci` first and do `npm run build-css` to get an optimized Tailwind css file. 
You can also start a watcher for that via `npm run css-gen`.

## Random Screenshot

![Screenshot of the app](img.png "I admit that I'm too lazy to add enough fake data for a better screenshot")

The page above runs the query ` SELECT * FROM 'resources' WHERE ((SELECT Count(*) FROM resource_tags rt WHERE rt.tag_id IN (1) AND rt.resource_id = resources.id) = 1) AND JSON_EXTRACT('meta',"$.silliness") > 7.000000 LIMIT 10`
Or something like that, depending on if you are using postgres or sqlite.

## maybe do

- [ ] Make note categories work
- [X] Sorting lists
- [X] Inline add for tags, groups, categories, and so on
- [ ] relationship editor with multiple groups (m x n, creating m * n relationships, and their reverse when applicable)
- [ ] bulk editor for lists
- [ ] Breadcrumbs for groups
- [X] video preview generation
- [ ] faster image previews with libvips?
- [X] support multiple file system attach points
- [ ] some integration tests, perhaps behavioral test
- [ ] importers like perkeep? maybe.
- [ ] sync could be interesting
