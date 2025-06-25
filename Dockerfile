FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    wget \
    gnupg \
    curl \
    lsb-release \
    software-properties-common \
    ca-certificates

RUN curl -sLf https://dl.redpanda.com/nzc4ZYQK3WRGd9sy/redpanda/cfg/setup/bash.deb.sh | bash \
 && apt-get update \
 && apt-get install -y redpanda


COPY boggle-backend/boggle-backend /usr/local/bin/boggle-backend
# COPY boggle-kafka/boggle-kafka /usr/local/bin/boggle-kafka
COPY redpanda.yaml /etc/redpanda/redpanda.yaml
RUN chmod +x /usr/local/bin/boggle-backend 

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

WORKDIR /var/lib/redpanda
EXPOSE 9092 5050

CMD ["/entrypoint.sh"]