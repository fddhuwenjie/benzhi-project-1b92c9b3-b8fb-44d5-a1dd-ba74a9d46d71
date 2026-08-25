package main

const defaultListenAddress = "127.0.0.1:19081"

func loopbackAddress(port string) string { return "127.0.0.1:" + port }
