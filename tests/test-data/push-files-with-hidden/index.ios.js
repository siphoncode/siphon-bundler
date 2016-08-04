'use strict';

var React = require('react-native');
var {
  AppRegistry,
  StyleSheet,
  View,
  Text,
  ScrollView,
  Image
} = React;

var App = React.createClass({
  render: function() {
    return (
      <Text>Hello.</Text>
    );
  }
});

AppRegistry.registerComponent('App', () => App);
