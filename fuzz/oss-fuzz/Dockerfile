FROM gcr.io/oss-fuzz-base/base-builder-go

# The repo vendors all dependencies, so the clone below is the only fetch.
RUN git clone --depth 1 https://github.com/Quad4-Software/Reticulum-Go $SRC/reticulum-go
WORKDIR $SRC/reticulum-go
COPY build.sh $SRC/build.sh
