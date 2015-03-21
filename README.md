Gogallery is a simple web server which allows to browse images in a convenient way with the help of tags (most of the behavior is copied from fukung.net). Clicking on an image opens another one randomly.

It is written in Go, and uses sqlite to store the tags. It also uses imagemagick's convert to generate the thumbnails, although that will change when a go native solution for resizing images has matured enough. 

{{{
usage: 
	 gogallery tag dir tagname
	 gogallery deltag tagname 
	 gogallery 
  -conf="": json conf file to send email alerts
  -h=false: show this help
  -host="localhost:8080": listening port and hostname that will appear in the urls
}}}

All images are initially tagged under "all", so 
`http://host/tag/all` will display them all.
Or simply start with `http://host/random`.

It can also be used for quick browsing of local images; just run *gogallery -init* in the images dir and go to http://localhost:8080/tag/all in your browser.

*Disclaimer*: this is just a toy. It has bugs, and lacks basic security features. Heck, it probably isn't safe against sql injections, so do not use it for anything serious. Maybe someday I'll clean it up but in the meantime I'll just maintain it so that it builds and starts.
Actually, I'll probably never touch it again. Just keeping it for history.
