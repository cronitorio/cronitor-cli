FROM debian:stretch-slim
RUN apt update
RUN apt install strace