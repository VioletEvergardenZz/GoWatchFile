#!/bin/bash

wget ${DOWNLOAD_FILE}

/opt/ParseHeapDump.sh ${FILE_NAME}.hprof org.eclipse.mat.api:suspects org.eclipse.mat.api:overview org.eclipse.mat.api:top_components

unzip -o "${FILE_NAME}"_Leak_Suspects.zip -d /opt/nginx/Leak_Suspects/
unzip -o "${FILE_NAME}"_System_Overview.zip -d /opt/nginx/System_Overview/
unzip -o "${FILE_NAME}"_Top_Components.zip -d /opt/nginx/Top_Components/