FROM node:20-slim

WORKDIR /usr/src/app

# Copy package files
COPY package*.json ./

# Install dependencies
RUN npm install

# Copy source code
COPY . .

# Expose ports
EXPOSE 3002
EXPOSE 3001

CMD ["node", "src/app.js"]
