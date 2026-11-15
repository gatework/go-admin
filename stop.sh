#!/bin/bash
killall motor # kill motor service
echo "stop motor success"
ps -aux | grep motor