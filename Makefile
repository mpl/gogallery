include $(GOROOT)/src/Make.inc
 
TARG=gogallery

GOFILES=\
	http.go \
	sql.go \
	html.go \
	main.go

include $(GOROOT)/src/Make.cmd
