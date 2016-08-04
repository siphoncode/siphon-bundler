
var React = require('react-native');
var {
  StyleSheet,
  View,
  Text,
  ScrollView
} = React;

var CardView = React.createClass({
  render: function() {
    return (
      <ScrollView style={styles.container} contentInset={{bottom: 34}}>
        <Text style={styles.title}>{this.props.title}</Text>
        {
          this.props.children.map(element => {
            return <View style={styles.card}>{element}</View>
          })
        }
      </ScrollView>
    );
  }
});

var styles = StyleSheet.create({
  container: {
    backgroundColor: '#eeeeee'
  },
  title: {
    fontSize: 30,
    color: '#919191',
    textAlign: 'center',
    paddingTop: 34
  },
  card: {
    backgroundColor: 'white',
    marginTop: 20,
    marginLeft: 14,
    marginRight: 14,
    borderRadius: 4,
    borderBottomWidth: 1.5,
    borderBottomColor: '#d7d7d7',
    overflow: 'hidden'
  }
});

module.exports = CardView;
