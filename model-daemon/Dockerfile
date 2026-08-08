FROM python:3.12-slim

WORKDIR /app

COPY server.py prompt.txt ./

RUN useradd --system --uid 10001 --create-home modeldaemon

USER modeldaemon

ENV LISTEN_ADDR=0.0.0.0:9000 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

EXPOSE 9000

HEALTHCHECK --interval=10s --timeout=2s --start-period=3s --retries=3 \
  CMD ["python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:9000/healthz', timeout=1).read()"]

CMD ["python", "server.py"]
