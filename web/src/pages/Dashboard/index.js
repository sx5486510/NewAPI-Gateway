import React from 'react';
import { Container } from 'semantic-ui-react';
import DashboardPanel from '../../components/DashboardPanel';
import Header from '../../components/Header';
import Footer from '../../components/Footer';

const Dashboard = () => (
    <>
        <Header />
        <Container>
            <DashboardPanel />
        </Container>
        <Footer />
    </>
);

export default Dashboard;
