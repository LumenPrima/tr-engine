FROM nvidia/cuda:12.4.1-cudnn-runtime-ubuntu22.04

RUN apt-get update && \
    apt-get install -y --no-install-recommends python3 python3-pip && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY requirements.txt .
RUN pip3 install --no-cache-dir -r requirements.txt

COPY server.py .

ENV WHISPER_MODEL=large-v3
ENV DEVICE=auto
ENV COMPUTE_TYPE=float16
ENV HOST=0.0.0.0
ENV PORT=8000

EXPOSE 8000

CMD ["python3", "server.py"]
