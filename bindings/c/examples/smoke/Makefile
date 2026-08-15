CC ?= gcc
ROOT := ../../../..
LIB := $(ROOT)/bin/librns.so
INC := $(ROOT)/include

.PHONY: all clean

all: librns-smoke

librns-smoke: main.c $(LIB)
	$(CC) -I$(INC) -L$(ROOT)/bin -Wl,-rpath,'$$ORIGIN/../../../../bin' -o $@ main.c -lrns

$(LIB):
	$(MAKE) -C $(ROOT) build-librns 2>/dev/null || (cd $(ROOT) && task build-librns)

clean:
	rm -f librns-smoke
