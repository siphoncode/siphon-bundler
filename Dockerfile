FROM golang:1.5.1

# Dependencies
RUN apt-get update
RUN apt-get install -q -y --force-yes git curl wget
RUN apt-get install -q -y --force-yes python3 python-virtualenv
RUN apt-get install -q -y --force-yes python3-setuptools
RUN easy_install3 pip

# Set up environment for nvm+node
ENV NVM_DIR /usr/local/nvm
ENV NODE_VERSION 4.2.1
ENV NODE_PATH $NVM_DIR/v$NODE_VERSION/lib/node_modules
ENV PATH $NVM_DIR/versions/node/v$NODE_VERSION/bin:$PATH

# Install nvm+node
RUN curl https://raw.githubusercontent.com/creationix/nvm/v0.29.0/install.sh | bash
RUN cat $NVM_DIR/nvm.sh >> install-node.sh
RUN echo "nvm install $NODE_VERSION" >> install-node.sh
RUN echo "nvm alias default $NODE_VERSION" >> install-node.sh
RUN echo "nvm use default" >> install-node.sh
RUN sh install-node.sh

# Install the packager
ENV PACKAGER_COMMIT 895570e
ENV BITBUCKET_API_KEY CHANGEME
RUN mkdir -p /code
RUN git clone https://getsiphon:${BITBUCKET_API_KEY}@bitbucket.org/getsiphon/siphon-packager.git /code/siphon-packager
WORKDIR /code/siphon-packager
RUN git checkout ${PACKAGER_COMMIT}
RUN python3 -m pip install -r requirements.txt
ENV PYTHONIOENCODING UTF-8
RUN chmod +x siphon-packager.py siphon-packager-install.py
WORKDIR /

# Put it on our PATH and install the packager's own dependencies
ENV PATH /code/siphon-packager:$PATH
RUN cat $NVM_DIR/nvm.sh >> install-packager.sh
RUN echo "siphon-packager-install.py" >> install-packager.sh
RUN sh install-packager.sh

# SSL keys
RUN mkdir -p /code/.keys
ADD deployment/keys/ /code/.keys/
ADD deployment/entrypoint.sh /code/entrypoint.sh

# Install the bundler
RUN mkdir -p /code/siphon-bundler
ADD src /code/siphon-bundler/src
ADD bundler.sh /code/siphon-bundler/
RUN chmod +x /code/siphon-bundler/bundler.sh
RUN chmod +x /code/siphon-packager/packager-daemon.sh
RUN chmod +x /code/entrypoint.sh

WORKDIR /code
ENTRYPOINT ./entrypoint.sh
