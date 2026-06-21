FROM nginx:1.27-alpine

ENV BACKEND_UPSTREAM=http://youban-server:8000

COPY dist /usr/share/nginx/html/admin
COPY nginx.conf.template /etc/nginx/templates/default.conf.template

EXPOSE 80
