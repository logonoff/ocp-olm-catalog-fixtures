FROM quay.io/operator-framework/opm:v1.74.0
ARG CATALOG=source
COPY dist/catalogs/${CATALOG}/ /configs/
LABEL operators.operatorframework.io.index.configs.v1=/configs/
ENTRYPOINT ["/bin/opm"]
CMD ["serve", "/configs"]
