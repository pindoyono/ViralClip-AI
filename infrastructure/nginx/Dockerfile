FROM nginx:1.25-alpine

RUN rm /etc/nginx/conf.d/default.conf

COPY nginx.conf /etc/nginx/nginx.conf
COPY conf.d/ /etc/nginx/conf.d/

RUN mkdir -p /var/www/certbot /etc/nginx/ssl

EXPOSE 80 443 8090

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD wget -qO- http://localhost:8090/health || exit 1

CMD ["nginx", "-g", "daemon off;"]
