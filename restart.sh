#!/bin/bash
echo "go build"
go mod tidy
go build -o motor main.go
chmod +x ./motor
echo "kill motor service"
killall motor # kill motor service
nohup ./motor server -c=config/settings.dev.yml >> access.log 2>&1 & #后台启动服务将日志写入access.log文件
echo "run motor success"
ps -aux | grep motor
