#final stage
FROM ubuntu:noble
ENV SOCKET_FILE_PATH=''
ENV HEALTH_CHECK_URL=''
ENV CRON_SCHEDULER_CONFIG=''
RUN apt-get update && apt-get install -y libgpgme-dev
RUN apt-get install -y ca-certificates
ADD build/containermon /exec/containermon
RUN chmod +x /exec/containermon
RUN chmod 777 /exec/containermon
ENTRYPOINT ["/bin/sh", "-c", "/exec/containermon"]
LABEL Name=containermon Version=1.0