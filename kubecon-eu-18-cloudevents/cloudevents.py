# Copyright 2017 The Nuclio Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

import json
import logging
import os
import os.path
import random
import re
import requests
import shutil
import tarfile
import traceback
import threading
import tweepy
import inflect

import numpy as np
import tensorflow as tf


def handler(context, event):

    # we're going to need a unique temporary location to handle each event,
    # as we download a file as part of each function invocation
    temp_dir = Helpers.create_temporary_dir(context, event)

    # wrap everything with error handling such that any exception raised
    # at any point will still return a proper response
    try:

        # if we're not ready to handle this request yet, deny it
        if not FunctionState.done_loading:
            context.logger.warn_with('Model data not done loading yet, denying request')
            raise NuclioResponseError('Model data not loaded yet, cannot serve this request',
                                      requests.codes.service_unavailable)

        # nuclio will automatically handle CloudEvent formatted events and create a `dict`, assuming 
        # Content-Type is application/cloudevents+json. However, some providers don't set this header 
        # so we need to parse the JSON here in the else clause
        if isinstance(event.body, dict):
            parsed_body = event.body
            eventType = event.type
        else:
            parsed_body = json.loads(event.body.decode('utf-8').strip())
            eventType = parsed_body['eventType']
            parsed_body = parsed_body['data']

        context.logger.info_with('Got event', kind=eventType)

        if eventType == 'aws.s3.object.created':
            image_url = 'https://s3.amazonaws.com/{0}/{1}'.format(
                parsed_body['bucket']['name'], 
                parsed_body['object']['key'])
        elif eventType == 'Microsoft.Storage.BlobCreated':
            image_url = parsed_body['url']
        else:
            context.logger.warn_with('Unsupported event type', kind=eventType)
            return    

        context.logger.info_with('Got image URL', image_url=image_url)
        image_url = image_url.strip()

        # download the image to our temporary location
        image_target_path = os.path.join(temp_dir, 'downloaded_image.jpg')
        Helpers.download_file(context, image_url, image_target_path)

        # run the inference on the image
        results = Helpers.run_inference(context, image_target_path, 5, 0)

        # if we didn't get a result, error!
        if not results:
            raise NuclioResponseError('Sorry, couldn\'t figure out what this is... Not very confident.',
                                      requests.codes.service_unavailable)

        # produce a tweet!
        tweet_url = Helpers.tweet(context, results, image_target_path)

        # return a response with the result
        return context.Response(body='Tweet sent out! Go to {0}'.format(tweet_url),
                                headers={},
                                content_type='text/plain',
                                status_code=requests.codes.ok)

    # convert any NuclioResponseError to a response to be returned from our handler.
    # the response's description and status will appropriately convey the underlying error's nature
    except NuclioResponseError as error:
        return error.as_response(context)

    # if anything we didn't count on happens, respond with internal server error
    except Exception as error:
        context.logger.warn_with('Unexpected error occurred, responding with internal server error',
                                 exc=str(error))

        message = 'Unexpected error occurred: {0}\n{1}'.format(error, traceback.format_exc())
        return NuclioResponseError(message).as_response(context)

    # clean up after ourselves regardless of whether we succeeded or failed
    finally:
        shutil.rmtree(temp_dir)


class NuclioResponseError(Exception):

    def __init__(self, description, status_code=requests.codes.internal_server_error):
        self._description = description
        self._status_code = status_code

    def as_response(self, context):
        return context.Response(body=self._description,
                                headers={},
                                content_type='text/plain',
                                status_code=self._status_code)


class FunctionState(object):
    """
    This class has classvars that are set by methods invoked during file import,
    such that handler invocations can re-use them.
    """

    # the object through which we send out tweets with results when invoked
    twitter_api = None

    # holds the TensorFlow graph def
    graph = None

    # holds the node id to human string mapping
    node_lookup = None

    # holds a boolean indicating if we're ready to handle an invocation or haven't finished yet
    done_loading = False


class Paths(object):

    # the directory in the deployed function container where the data model is saved
    model_dir = os.getenv('MODEL_DIR', '/tmp/tfmodel/')

    # paths of files within the model archive used to create the graph
    label_lookup_path = os.path.join(model_dir,
                                     os.getenv('LABEL_LOOKUP_FILENAME',
                                               'imagenet_synset_to_human_label_map.txt'))

    uid_lookup_path = os.path.join(model_dir,
                                   os.getenv('UID_LOOKUP_FILENAME',
                                             'imagenet_2012_challenge_label_map_proto.pbtxt'))

    graph_def_path = os.path.join(model_dir,
                                  os.getenv('GRAPH_DEF_FILENAME',
                                            'classify_image_graph_def.pb'))


class Helpers(object):

    @staticmethod
    def create_temporary_dir(context, event):
        """
        Creates a uniquely-named temporary directory (based on the given event's id) and returns its path.
        """
        temp_dir = '/tmp/nuclio-event-{0}'.format(event.id)
        os.makedirs(temp_dir)

        context.logger.debug_with('Created temporary directory', path=temp_dir)

        return temp_dir

    @staticmethod
    def run_inference(context, image_path, num_predictions, confidence_threshold):
        """
        Runs inference on the image in the given path.
        Returns a list of up to N=num_prediction tuples (prediction human name, confidence score).
        Only takes predictions whose confidence score meets the provided confidence threshold.
        """

        # read the image binary data
        with tf.gfile.FastGFile(image_path, 'rb') as f:
            image_data = f.read()

        # run the graph's softmax tensor on the image data
        with tf.Session(graph=FunctionState.graph) as session:
            softmax_tensor = session.graph.get_tensor_by_name('softmax:0')
            predictions = session.run(softmax_tensor, {'DecodeJpeg/contents:0': image_data})
            predictions = np.squeeze(predictions)

        results = []

        # take the num_predictions highest scoring predictions
        top_predictions = reversed(predictions.argsort()[-num_predictions:])

        # look up each predicition's human-readable name and add it to the
        # results if it meets the confidence threshold
        for node_id in top_predictions:
            name = FunctionState.node_lookup[node_id]

            score = float(predictions[node_id])
            meets_threshold = score > confidence_threshold

            # tensorflow's float32 must be converted to float before logging, not JSON-serializable
            context.logger.info_with('Found prediction',
                                     name=name,
                                     score=score,
                                     meets_threshold=meets_threshold)

            if meets_threshold:
                results.append((name, score))

        return results

    @staticmethod
    def tweet(context, results, image_path=None):
        """
        Tweets out the top result and the confidence percentage. Returns the tweet's URL.
        """

        # prepare the string to tweet
        top_prediction = results[0]
        percentage_confidence = '{0}%'.format(int(top_prediction[1] * 100))
        first_noun = top_prediction[0].split(', ')[0].lower()

        # some nouns are A plane, others are AN airplane. some nouns are "DRUMS" and will just embarrass us.
        if FunctionState.inflect_engine.singular_noun(first_noun) is False:
            noun_prefix = 'an ' if first_noun[0] in ['a', 'e', 'i', 'o', 'u'] else 'a '
        else:
            noun_prefix = ''

        # add some variation
        tweet_variants = [
            'That\'s {0}{1}, I\'m like {2} sure.',
            'I found {0}{1}! At least, I\'m {2} sure I did.',
            'Is that {0}{1}? I think it is! About {2} sure.',
            'I\'m only {2} confident in saying this, but what {0}{1}!',
            'Ooooh, {0}{1}... I mean, I think it is. With {2} of certainty.'
        ]

        contents = random.choice(tweet_variants).format(noun_prefix, first_noun, percentage_confidence)
        context.logger.info_with('Tweeting out', tweet_contents=contents)

        # tweet it using twitter's API
        if image_path is not None:
            status = FunctionState.twitter_api.update_with_media(image_path, status=contents)
        else:
            status = FunctionState.twitter_api.update_status(contents)

        # return the tweet URL
        return 'https://twitter.com/{0}/status/{1}'.format(status.user.screen_name, status.id_str)

    @staticmethod
    def on_import():
        """
        This function is called when the file is imported, so that model data
        is loaded to memory only once per function deployment.
        """

        # set twitter up
        FunctionState.twitter_api = Helpers.initialize_tweepy()

        # set inflection up
        FunctionState.inflect_engine = inflect.engine()

        # load the graph def from trained model data
        FunctionState.graph = Helpers.load_graph_def()

        # load the node ID to human-readable string mapping
        FunctionState.node_lookup = Helpers.load_node_lookup()

        # signal that we're ready
        FunctionState.done_loading = True

    @staticmethod
    def initialize_tweepy():
        """
        Logs into our twitter account for tweepy to work. Returns a tweepy API object.
        """

        # fetch authentication details from environment
        consumer_key = os.getenv('TWITTER_CONSUMER_KEY',
                                 '<REPLACE ME>')

        consumer_secret = os.getenv('TWITTER_CONSUMER_SECRET',
                                    '<REPLACE ME>')

        access_token = os.getenv('TWITTER_ACCESS_TOKEN',
                                 '<REPLACE ME>')

        access_token_secret = os.getenv('TWITTER_ACCESS_TOKEN_SECRET',
                                        '<REPLACE ME>')

        # provide the application oauth credentials
        auth = tweepy.OAuthHandler(consumer_key, consumer_secret)

        # set the tweeting user's access token
        auth.set_access_token(access_token, access_token_secret)

        # return the API object with the authentication
        return tweepy.API(auth)

    @staticmethod
    def load_graph_def():
        """
        Imports the GraphDef data into TensorFlow's default graph, and returns it.
        """

        # verify that the declared graph def file actually exists
        if not tf.gfile.Exists(Paths.graph_def_path):
            raise NuclioResponseError('Failed to find graph def file', requests.codes.service_unavailable)

        # load the TensorFlow GraphDef
        with tf.gfile.FastGFile(Paths.graph_def_path, 'rb') as f:
            graph_def = tf.GraphDef()
            graph_def.ParseFromString(f.read())

            tf.import_graph_def(graph_def, name='')

        return tf.get_default_graph()

    @staticmethod
    def load_node_lookup():
        """
        Composes the mapping between node IDs and human-readable strings. Returns the composed mapping.
        """

        # load the mappings from which we can build our mapping
        string_uid_to_labels = Helpers._load_label_lookup()
        node_id_to_string_uids = Helpers._load_uid_lookup()

        # compose the final mapping of integer node ID to human-readable string
        result = {}
        for node_id, string_uid in node_id_to_string_uids.items():
            label = string_uid_to_labels.get(string_uid)

            if label is None:
                raise NuclioResponseError('Failed to compose node lookup')

            result[node_id] = label

        return result

    @staticmethod
    def download_file(context, url, target_path):
        """
        Downloads the given remote URL to the specified path.
        """
        # make sure the target directory exists
        os.makedirs(os.path.dirname(target_path), exist_ok=True)
        try:
            with requests.get(url, stream=True) as response:
                response.raise_for_status()
                with open(target_path, 'wb') as f:
                    for chunk in response.iter_content(chunk_size=8192):
                        if chunk:
                            f.write(chunk)
        except Exception as error:
            if context is not None:
                context.logger.warn_with('Failed to download file',
                                         url=url,
                                         target_path=target_path,
                                         exc=str(error))
            raise NuclioResponseError('Failed to download file: {0}'.format(url),
                                      requests.codes.service_unavailable)
        if context is not None:
            context.logger.info_with('Downloaded file successfully',
                                     size_bytes=os.stat(target_path).st_size,
                                     target_path=target_path)

    @staticmethod
    def _load_label_lookup():
        """
        Loads and parses the mapping between string UIDs and human-readable strings. Returns the parsed mapping.
        """

        # verify that the declared label lookup file actually exists
        if not tf.gfile.Exists(Paths.label_lookup_path):
            raise NuclioResponseError('Failed to find Label lookup file', requests.codes.service_unavailable)

        # load the raw mapping data
        with tf.gfile.GFile(Paths.label_lookup_path) as f:
            lookup_lines = f.readlines()

        result = {}

        # parse the raw data to a mapping between string UIDs and labels
        # each line is expected to look like this:
        # n12557064     kidney bean, frijol, frijole
        line_pattern = re.compile(r'(n\d+)\s+([ \S,]+)')

        for line in lookup_lines:
            matches = line_pattern.findall(line)

            # extract the uid and label from the matches
            # in our example, uid will be "n12557064" and label will be "kidney bean, frijol, frijole"
            uid = matches[0][0]
            label = matches[0][1]

            # insert the UID and label to our mapping
            result[uid] = label

        return result

    @staticmethod
    def _load_uid_lookup():
        """
        Loads and parses the mapping between node IDs and string UIDs. Returns the parsed mapping.
        """

        # verify that the declared uid lookup file actually exists
        if not tf.gfile.Exists(Paths.uid_lookup_path):
            raise NuclioResponseError('Failed to find UID lookup file', requests.codes.service_unavailable)

        # load the raw mapping data
        with tf.gfile.GFile(Paths.uid_lookup_path) as f:
            lookup_lines = f.readlines()

        result = {}

        # parse the raw data to a mapping between integer node IDs and string UIDs
        # this file is expected to contains entries such as this:
        #
        # entry
        # {
        #   target_class: 443
        #   target_class_string: "n01491361"
        # }
        #
        # to parse it, we'll iterate over the lines, and for each line that begins with "  target_class:"
        # we'll assume that the next line has the corresponding "target_class_string"
        for i, line in enumerate(lookup_lines):

            # we found a line that starts a new entry in our mapping
            if line.startswith('  target_class:'):
                # target_class represents an integer value for node ID - convert it to an integer
                target_class = int(line.split(': ')[1])

                # take the string UID from the next line,
                # and clean up the quotes wrapping it (and the trailing newline)
                next_line = lookup_lines[i + 1]
                target_class_string = next_line.split(': ')[1].strip('"\n ')

                # insert the node ID and string UID to our mapping
                result[target_class] = target_class_string

        return result


# perform the loading in another thread to not block import - the function
# handler will gracefully decline requests until we're ready to handle them
t = threading.Thread(target=Helpers.on_import)
t.start()
