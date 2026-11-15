#!/bin/bash
kubectl create ns motor
kubectl create configmap settings-admin --from-file=../../config/settings.yml -n motor
