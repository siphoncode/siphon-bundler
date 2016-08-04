
var React = require('react-native');
var {
  StyleSheet,
  View,
  ProgressViewIOS
} = React;


var AnimatedProgressView = React.createClass({
  getInitialState: function() {
    return {progress: 0};
  },

  componentDidMount: function() {
    this.updateProgress();
  },

  updateProgress() {
    var progress = this.state.progress + 0.01;
    this.setState({progress: progress});
    requestAnimationFrame(() => this.updateProgress());
  },

  getProgress: function(offset) {
    var progress = this.state.progress + offset;
    return Math.sin(progress % Math.PI) % 1;
  },

  render: function() {
    return (
      <View style={styles.container}>
        <ProgressViewIOS
          style={styles.progress}
          progressTintColor="purple"
          progress={this.getProgress(0.2)}
        />
        <ProgressViewIOS
          style={styles.progress}
          progressTintColor="red"
          progress={this.getProgress(0.4)}
        />
        <ProgressViewIOS
          style={styles.progress}
          progressTintColor="orange"
          progress={this.getProgress(0.6)}
        />
        <ProgressViewIOS
          style={styles.progress}
          progressTintColor="yellow"
          progress={this.getProgress(0.8)}
        />
      </View>
    );
  }
});

var styles = StyleSheet.create({
  container: {
    backgroundColor: 'white',
    marginTop: 10,
    marginBottom: 10
  },
  progress: {
    margin: 10
  }
});

module.exports = AnimatedProgressView;
