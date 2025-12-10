#!/bin/bash

if ! command -v go &> /dev/null
then
    echo "Go is not installed. Please install Go first."
    exit 1
fi

if ! command -v zip &> /dev/null
then
    echo "zip is not installed. Please install zip first."
    exit 1
fi

if ! command -v tar &> /dev/null
then
    echo "tar is not installed. Please install tar first."
    exit 1
fi

start_time=$(date +%s)

if [ ! -d "bin" ]; then
    mkdir bin
else
    rm -f bin/*
fi

rm -f upx.log

for os in windows linux darwin
do
    for arch in amd64 arm64
    do
        echo "Building nativedb_${os}_${arch}..."
        GOOS=$os GOARCH=$arch go build -o bin/nativedb_${os}_${arch}$(if [ "$os" = "windows" ]; then echo ".exe"; fi) -ldflags "-w -s"
    done
done

if command -v upx &> /dev/null
then
    for os in windows linux darwin
    do
        for arch in amd64 arm64
        do
            if [ "$os" != "darwin" ] && [ "$arch" != "arm64" ]; then
                echo "Compressing nativedb_${os}_${arch}..."
                upx -9 bin/nativedb_${os}_${arch}$(if [ "$os" = "windows" ]; then echo ".exe"; fi) > upx.log
            fi
        done
    done
else
    echo "UPX is not installed. Skipping compression."
fi

for os in windows linux darwin
do
    for arch in amd64 arm64
    do
        echo "Packaging nativedb_${os}_${arch}..."
        if [ "$os" = "windows" ]; then
            zip -j bin/nativedb_${os}_${arch}.zip bin/nativedb_${os}_${arch}.exe
            rm bin/nativedb_${os}_${arch}.exe
        else
            tar -czvf bin/nativedb_${os}_${arch}.tar.gz -C bin nativedb_${os}_${arch}
            rm bin/nativedb_${os}_${arch}
        fi
    done
done

end_time=$(date +%s)
elapsed_time=$((end_time - start_time))
echo "Build and compress completed in ${elapsed_time} seconds."
