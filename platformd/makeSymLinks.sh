#!/bin/bash

if [ "$1" == "onlp" ]; then
	cd pluginManager/onlp/libs
	ln -sf libonlp.so.1 libonlp.so
elif [ "$1" == "openBMC" ]; then
	echo "Build Target is OpenBMC"
fi
